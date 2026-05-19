/*
 * Copyright Daytona Platforms Inc.
 * SPDX-License-Identifier: AGPL-3.0
 */

import { MigrationInterface, QueryRunner } from 'typeorm'

export class Migration1778800000000 implements MigrationInterface {
  name = 'Migration1778800000000'

  public async up(queryRunner: QueryRunner): Promise<void> {
    await queryRunner.query(`ALTER TABLE "volume" ADD "currentStorageMb" float`)
    await queryRunner.query(`ALTER TABLE "volume" ADD "storageCheckedAt" TIMESTAMP WITH TIME ZONE`)
  }

  public async down(queryRunner: QueryRunner): Promise<void> {
    await queryRunner.query(`ALTER TABLE "volume" DROP COLUMN "storageCheckedAt"`)
    await queryRunner.query(`ALTER TABLE "volume" DROP COLUMN "currentStorageMb"`)
  }
}
