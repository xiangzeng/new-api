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

/**
 * One user running a reseller center, as the operator sees it.
 *
 * `user_status` is the account state and `status` is the reseller profile
 * state; a disabled account can still own an active profile, so the two are
 * reported separately.
 */
export interface ResellerRosterItem {
  user_id: number
  username: string
  display_name: string
  user_status: number
  status: string
  customer_count: number
  pending_commission_quota: number
  available_commission_quota: number
  created_at: number
}
