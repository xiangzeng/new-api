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
import type { User } from '../types'

/**
 * Whether the user currently has 千人千面 per-user pricing turned on.
 *
 * The backend ships the raw `custom_pricing` JSON blob on the user row, so the
 * enabled flag has to be parsed defensively (legacy rows may hold invalid JSON).
 */
export function isCustomPricingEnabled(user: User): boolean {
  if (!user.custom_pricing) return false
  try {
    const pricing = JSON.parse(user.custom_pricing) as { enabled?: boolean }
    return pricing.enabled === true
  } catch {
    return false
  }
}
