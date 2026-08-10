/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

/** Partner application status values mirrored from `model.OpenApp`. */
export const OPEN_APP_STATUS = {
  ENABLED: 1,
  DISABLED: 2,
} as const

/** One row of `GET /api/open-app/`. The secret itself is never returned. */
export interface OpenApp {
  id: number
  app_id: string
  /** Trailing characters of the secret, enough to tell two credentials apart. */
  secret_hint: string
  name: string
  status: number
  /** Newline separated CIDR/IP allow list; empty means no source restriction. */
  allowed_ips: string
  /** Per-minute exchange cap for this partner; 0 falls back to the global value. */
  exchange_rate_limit: number
  created_time: number
  last_used_time: number
}

export interface OpenAppRequest {
  name: string
  allowed_ips: string
  status: number
  exchange_rate_limit: number
}

export interface OpenAppApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

/** Create and reset both return the clear-text secret exactly once. */
export interface OpenAppSecretPayload {
  app: OpenApp
  app_secret: string
}
