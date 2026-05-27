# Copyright Daytona Platforms Inc.
# SPDX-License-Identifier: Apache-2.0

# frozen_string_literal: true

require 'json'
require 'logger'

require 'daytona_api_client'
require 'daytona_toolbox_api_client'
require 'toml'
require 'websocket-client-simple'

require_relative 'sdk/version'
require_relative 'config'
require_relative 'otel'
require_relative 'common/charts'
require_relative 'common/code_interpreter'
require_relative 'common/code_language'
require_relative 'common/daytona'
require_relative 'common/file_system'
require_relative 'common/image'
require_relative 'common/git'
require_relative 'common/process'
require_relative 'common/pty'
require_relative 'common/resources'
require_relative 'common/response'
require_relative 'common/snapshot'
require_relative 'code_interpreter'
require_relative 'computer_use'
require_relative 'daytona'
require_relative 'file_system'
require_relative 'git'
require_relative 'lsp_server'
require_relative 'object_storage'
require_relative 'sandbox'
require_relative 'snapshot_service'
require_relative 'util'
require_relative 'volume'
require_relative 'volume_service'
require_relative 'process'

module Daytona
  module Sdk
    API_ERROR_CLASSES = [DaytonaApiClient::ApiError, DaytonaToolboxApiClient::ApiError].freeze

    class Error < StandardError
      def initialize(message = nil, status_code: nil, code: nil, source: nil)
        super(message)
        @status_code = status_code
        @code = code
        @source = source
      end

      def status_code = @status_code || metadata_from_cause[:status_code]

      def code = @code || metadata_from_cause[:code]

      def source = @source || metadata_from_cause[:source]

      private

      def metadata_from_cause
        @metadata_from_cause ||= Sdk.api_error_details(cause)
      end
    end

    class TimeoutError < Error
      def initialize(message = nil, **kwargs)
        super(message, **kwargs)
      end
    end

    class AuthenticationError < Error
      def initialize(message = nil, **kwargs)
        super(message, **kwargs)
      end
    end

    class ForbiddenError < Error
      def initialize(message = nil, **kwargs)
        super(message, **kwargs)
      end
    end

    class NotFoundError < Error
      def initialize(message = nil, **kwargs)
        super(message, **kwargs)
      end
    end

    class ConflictError < Error
      def initialize(message = nil, **kwargs)
        super(message, **kwargs)
      end
    end

    class ValidationError < Error
      def initialize(message = nil, **kwargs)
        super(message, **kwargs)
      end
    end

    class RateLimitError < Error
      def initialize(message = nil, **kwargs)
        super(message, **kwargs)
      end
    end

    class ServerError < Error
      def initialize(message = nil, **kwargs)
        super(message, **kwargs)
      end
    end

    def self.wrap_error(error, prefix = nil)
      message = prefix ? "#{prefix}: #{error.message}" : error.message
      details = api_error_details(error)
      error_class_for_status(details[:status_code]).new(
        message,
        status_code: details[:status_code],
        code: details[:code],
        source: details[:source]
      )
    end

    def self.api_error_details(error)
      return {} unless API_ERROR_CLASSES.any? { |api_error_class| error.is_a?(api_error_class) }

      response_body = error.respond_to?(:response_body) ? error.response_body : nil
      data = parse_error_body(response_body)

      {
        status_code: error.respond_to?(:code) ? error.code : nil,
        code: data[:code],
        source: data[:source]
      }
    end

    def self.parse_error_body(response_body)
      return {} if response_body.nil? || response_body.empty?

      data = JSON.parse(response_body)
      return {} unless data.is_a?(Hash)

      {
        code: string_or_nil(data['code'] || data['error_code']),
        source: string_or_nil(data['source'])
      }
    rescue JSON::ParserError
      {}
    end

    def self.error_class_for_status(status_code)
      case status_code
      when 400 then ValidationError
      when 401 then AuthenticationError
      when 403 then ForbiddenError
      when 404 then NotFoundError
      when 409 then ConflictError
      when 429 then RateLimitError
      when 500..599 then ServerError
      else Error
      end
    end

    def self.string_or_nil(value)
      value.is_a?(String) && !value.empty? ? value : nil
    end

    def self.logger = @logger ||= Logger.new($stdout, level: Logger::INFO)
  end
end
