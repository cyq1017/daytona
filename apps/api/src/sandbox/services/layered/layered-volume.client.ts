/*
 * Copyright 2025 Daytona Platforms Inc.
 * SPDX-License-Identifier: AGPL-3.0
 */

import { Injectable, Logger, ServiceUnavailableException } from '@nestjs/common'
import { TypedConfigService } from '../../../config/typed-config.service'
import {
  LayeredVolumeProvider,
  CreateDiskOptions,
  CreateDiskResult,
  MintMountKeyOptions,
  MintMountKeyResult,
} from './layered-volume.provider'

// Default per-region control-plane URLs used by the layered volume backend.
//
// Each region has its own control-plane host; disks live in exactly one
// region and must be created against the matching host. The mapping below
// can be overridden per-region with `LAYERED_CONTROL_URL_<REGION>` env vars
// (with `-` replaced by `_`, e.g. `LAYERED_CONTROL_URL_AWS_US_EAST_1`) when a
// future region or staging endpoint becomes available.
const DEFAULT_CONTROL_URLS: Record<string, string> = {
  'aws-us-east-1': 'https://control.green.us-east-1.aws.prod.archil.com',
  'aws-eu-west-1': 'https://control.green.eu-west-1.aws.prod.archil.com',
  'aws-us-west-2': 'https://control.green.us-west-2.aws.prod.archil.com',
  'gcp-us-central1': 'https://control.blue.us-central1.gcp.prod.archil.com',
}

// Re-export interface types for consumers that import from this file.
export type {
  DiskMount as LayeredDiskMount,
  CreateDiskOptions as CreateLayeredDiskOptions,
} from './layered-volume.provider'
export type { CreateDiskResult as LayeredDisk } from './layered-volume.provider'

interface ApiResponseEnvelope<T> {
  success: boolean
  error?: string
  data?: T
}

interface CreateDiskResponseData {
  diskId: string
  authorizedUsers?: Array<{
    type?: string
    token?: string
    identifier?: string
    nickname?: string
  }>
}

interface AddDiskUserResponseData {
  type?: string
  token?: string
  identifier?: string
  nickname?: string
}

export type { MintMountKeyResult, MintMountKeyOptions } from './layered-volume.provider'

@Injectable()
export class LayeredVolumeClient implements LayeredVolumeProvider {
  private readonly logger = new Logger(LayeredVolumeClient.name)
  private readonly apiKey?: string
  private readonly defaultRegion: string

  constructor(private readonly configService: TypedConfigService) {
    this.apiKey = this.configService.get('layered.apiKey')
    this.defaultRegion = this.configService.get('layered.defaultRegion') || 'aws-us-east-1'
  }

  // Whether the layered control-plane integration is configured. The
  // layered volume backend requires this; volumes scheduled for that
  // backend will fail with a clear error when it isn't.
  isConfigured(): boolean {
    return Boolean(this.apiKey)
  }

  getDefaultRegion(): string {
    return this.defaultRegion
  }

  async createDisk(opts: CreateDiskOptions): Promise<CreateDiskResult> {
    this.assertConfigured()
    const region = opts.region || this.defaultRegion
    const baseUrl = this.resolveControlUrl(region)

    const res = await this.request<CreateDiskResponseData>(`${baseUrl}/api/disks`, {
      method: 'POST',
      body: JSON.stringify({
        name: opts.name,
        // The control plane takes a list of mounts; we always provision
        // exactly one S3 mount per disk so the disk is a 1:1 view of a
        // Daytona-owned bucket (optionally scoped to a prefix). No
        // vendor-managed storage path is supported.
        mounts: [opts.mount],
      }),
    })

    if (!res.diskId) {
      throw new Error('createDisk response missing diskId')
    }

    // Pull the auto-generated token user the API provisions on disk
    // creation. We only ever provision token-based users for Daytona
    // volumes; AWS STS users are out of scope here.
    const tokenUser = res.authorizedUsers?.find((u) => u.type === 'token' && u.token)
    if (!tokenUser?.token) {
      throw new Error(
        `createDisk response did not include a generated token (diskId=${res.diskId}). ` +
          `The disk exists but is not mountable; delete it manually or open a support ticket.`,
      )
    }

    return {
      diskId: res.diskId,
      region,
      mountToken: tokenUser.token,
    }
  }

  // Deletes a layered disk and all its associated resources. Treats 404 as
  // success so that retries after a partial delete are safe.
  async deleteDisk(diskId: string, region: string): Promise<void> {
    this.assertConfigured()
    const baseUrl = this.resolveControlUrl(region)

    await this.request<unknown>(`${baseUrl}/api/disks/${encodeURIComponent(diskId)}`, {
      method: 'DELETE',
      treat404AsOk: true,
    })
  }

  async mintMountKey(opts: MintMountKeyOptions): Promise<MintMountKeyResult> {
    this.assertConfigured()
    const baseUrl = this.resolveControlUrl(opts.region)

    const res = await this.request<AddDiskUserResponseData>(
      `${baseUrl}/api/disks/${encodeURIComponent(opts.diskId)}/users`,
      {
        method: 'POST',
        body: JSON.stringify({
          type: 'token',
          nickname: opts.nickname,
        }),
      },
    )

    if (!res?.token) {
      throw new Error(`mintMountKey response missing token for disk ${opts.diskId}`)
    }
    if (!res?.identifier) {
      throw new Error(`mintMountKey response missing identifier for disk ${opts.diskId}`)
    }
    return { token: res.token, identifier: res.identifier }
  }

  // Revokes a previously minted token from a disk. Treats 404 as success so
  // double-revokes (e.g. after partial sandbox destroy) are safe.
  async revokeMountKey(diskId: string, region: string, identifier: string): Promise<void> {
    this.assertConfigured()
    const baseUrl = this.resolveControlUrl(region)

    const params = new URLSearchParams({ identifier })
    await this.request<unknown>(`${baseUrl}/api/disks/${encodeURIComponent(diskId)}/users/token?${params.toString()}`, {
      method: 'DELETE',
      treat404AsOk: true,
    })
  }

  private assertConfigured(): void {
    if (!this.apiKey) {
      throw new ServiceUnavailableException(
        'Layered volume control plane is not configured. Set LAYERED_API_KEY to enable the layered volume backend.',
      )
    }
  }

  private resolveControlUrl(region: string): string {
    const overrideKey = `LAYERED_CONTROL_URL_${region.toUpperCase().replace(/-/g, '_')}`
    const override = process.env[overrideKey]
    if (override) {
      return override.replace(/\/$/, '')
    }
    const fallback = DEFAULT_CONTROL_URLS[region]
    if (!fallback) {
      throw new Error(
        `Unknown layered region "${region}". Set ${overrideKey} to its control-plane URL or pick a documented region.`,
      )
    }
    return fallback
  }

  private async request<T>(
    url: string,
    init: { method: 'GET' | 'POST' | 'DELETE'; body?: string; treat404AsOk?: boolean },
  ): Promise<T> {
    const response = await fetch(url, {
      method: init.method,
      headers: {
        Authorization: `key-${this.apiKey}`,
        'Content-Type': 'application/json',
      },
      body: init.body,
    })

    if (init.treat404AsOk && response.status === 404) {
      return undefined as T
    }

    let envelope: ApiResponseEnvelope<T>
    const raw = await response.text()
    try {
      envelope = raw ? (JSON.parse(raw) as ApiResponseEnvelope<T>) : { success: response.ok }
    } catch {
      throw new Error(
        `Layered ${init.method} ${url} returned non-JSON response (status ${response.status}): ${raw.slice(0, 200)}`,
      )
    }

    if (!response.ok || envelope.success === false) {
      const message = envelope.error || `${init.method} ${url} failed with status ${response.status}`
      throw new Error(`Layered control plane error: ${message}`)
    }

    return envelope.data as T
  }
}
