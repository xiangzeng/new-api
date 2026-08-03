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
/** Consumption aggregated over the selected time window (from the log database). */
export interface InvitationPeriodStats {
  period_quota: number
  period_request_count: number
  period_prompt_tokens: number
  period_completion_tokens: number
}

/** One inviter row of `GET /api/invitation/summary`. */
export interface InvitationSummary extends InvitationPeriodStats {
  inviter_id: number
  inviter_username: string
  inviter_display_name: string
  inviter_email: string
  inviter_deleted: boolean
  aff_code: string
  aff_quota: number
  aff_history_quota: number
  invitee_count: number
  invitee_total_used_quota: number
  invitee_total_request_count: number
}

/** One invitee row of `GET /api/invitation/invitees`. */
export interface InvitationInvitee extends InvitationPeriodStats {
  invitee_id: number
  username: string
  display_name: string
  email: string
  group: string
  status: number
  is_deleted: boolean
  quota: number
  used_quota: number
  request_count: number
}

/** Time window shared by the summary table and the invitee panel (unix seconds). */
export interface InvitationTimeRange {
  start_timestamp: number
  end_timestamp: number
}

export interface InvitationPageParams extends InvitationTimeRange {
  p: number
  page_size: number
  keyword?: string
}

export interface InvitationListResponse<T> {
  success: boolean
  message?: string
  data?: {
    items: T[]
    total: number
    page: number
    page_size: number
  }
}
