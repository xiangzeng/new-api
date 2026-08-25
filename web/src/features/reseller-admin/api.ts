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
import type {
  ResellerCustomer,
  ResellerEnvelope,
  ResellerPage,
} from '@/features/reseller/types'
import { api } from '@/lib/api'

import type {
  OpenResellerPayload,
  OpenResellerResult,
  ResellerRosterItem,
} from './types'

export async function listResellers(
  page: number,
  pageSize: number,
  keyword: string
): Promise<ResellerEnvelope<ResellerPage<ResellerRosterItem>>> {
  const res = await api.get('/api/reseller/admin/resellers', {
    params: { p: page, page_size: pageSize, keyword },
  })
  return res.data
}

/**
 * The operator's view of one reseller's customers. Same projection the reseller
 * sees in its own center, addressed by reseller id instead of by session.
 */
export async function listResellerCustomers(
  resellerId: number,
  page: number,
  pageSize: number
): Promise<ResellerEnvelope<ResellerPage<ResellerCustomer>>> {
  const res = await api.get(
    `/api/reseller/admin/resellers/${resellerId}/customers`,
    { params: { p: page, page_size: pageSize } }
  )
  return res.data
}

/**
 * Opens the reseller center for an account that never opened it itself.
 *
 * The endpoint is idempotent: reopening an already open center succeeds with
 * `created: false` rather than failing, so the caller reports what happened
 * instead of having to check first.
 */
export async function openResellerCenter(
  payload: OpenResellerPayload
): Promise<ResellerEnvelope<OpenResellerResult>> {
  const res = await api.post('/api/reseller/admin/resellers', payload, {
    skipErrorHandler: true,
  })
  return res.data
}
