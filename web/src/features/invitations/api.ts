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
import { api } from '@/lib/api'

import type {
  InvitationInvitee,
  InvitationListResponse,
  InvitationPageParams,
  InvitationSummary,
} from './types'

/**
 * Get paginated inviter summaries. `keyword` matches inviter id, username,
 * display name, email or invitation code.
 */
export async function getInvitationSummaries(
  params: InvitationPageParams
): Promise<InvitationListResponse<InvitationSummary>> {
  const res = await api.get('/api/invitation/summary', {
    params: {
      p: params.p,
      page_size: params.page_size,
      keyword: params.keyword ?? '',
      start_timestamp: params.start_timestamp,
      end_timestamp: params.end_timestamp,
    },
  })
  return res.data
}

/**
 * Get the paginated invitee list of a single inviter. `keyword` matches
 * invitee id, username, display name or email.
 */
export async function getInvitationInvitees(
  inviterId: number,
  params: InvitationPageParams
): Promise<InvitationListResponse<InvitationInvitee>> {
  const res = await api.get('/api/invitation/invitees', {
    params: {
      inviter_id: inviterId,
      p: params.p,
      page_size: params.page_size,
      keyword: params.keyword ?? '',
      start_timestamp: params.start_timestamp,
      end_timestamp: params.end_timestamp,
    },
  })
  return res.data
}
