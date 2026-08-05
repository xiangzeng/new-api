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
import { z } from 'zod'

import type { AdminPermissionMatrix } from '@/lib/admin-permissions'

// ============================================================================
// User Schema & Types
// ============================================================================

/** User status: 1 = enabled, 2 = disabled, 3+ = other states */
export const userStatusSchema = z.number()
export type UserStatus = z.infer<typeof userStatusSchema>

/** User role: 1 = common user, 10 = admin, 100 = root */
export const userRoleSchema = z.number()
export type UserRole = z.infer<typeof userRoleSchema>

export const userSchema = z.object({
  id: z.number(),
  username: z.string(),
  display_name: z.string(),
  password: z.string().optional(),
  github_id: z.string().optional(),
  oidc_id: z.string().optional(),
  wechat_id: z.string().optional(),
  telegram_id: z.string().optional(),
  email: z.string().optional(),
  quota: z.number(),
  used_quota: z.number(),
  request_count: z.number(),
  group: z.string(),
  aff_code: z.string().optional(),
  aff_count: z.number().optional(),
  aff_quota: z.number().optional(),
  aff_history_quota: z.number().optional(),
  inviter_id: z.number().optional(),
  linux_do_id: z.string().optional(),
  status: userStatusSchema,
  role: userRoleSchema,
  created_at: z.number().optional(),
  updated_at: z.number().optional(),
  last_login_at: z.number().optional(),
  DeletedAt: z.any().nullable().optional(),
  remark: z.string().optional(),
  custom_pricing: z.string().optional(),
  admin_permissions: z
    .record(z.string(), z.record(z.string(), z.boolean()))
    .optional(),
})
export type User = z.infer<typeof userSchema>

export const userListSchema = z.array(userSchema)

// ============================================================================
// API Request/Response Types
// ============================================================================

/** Generic API response */
export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export type UserSortBy =
  | 'id'
  | 'username'
  | 'quota'
  | 'group'
  | 'created_at'
  | 'last_login_at'

export type UserSortOrder = 'asc' | 'desc'

export interface GetUsersParams {
  p?: number
  page_size?: number
  sort_by?: UserSortBy
  sort_order?: UserSortOrder
}

export interface GetUsersResponse {
  success: boolean
  message?: string
  data?: {
    items: User[]
    total: number
    page: number
    page_size: number
  }
}

export interface SearchUsersParams {
  keyword?: string
  group?: string
  role?: string
  status?: string
  p?: number
  page_size?: number
  sort_by?: UserSortBy
  sort_order?: UserSortOrder
}

export interface UserFormData {
  username: string
  display_name: string
  password?: string
  role?: number // Only used when creating user
  quota?: number // Only used when updating user
  group?: string // Only used when updating user
  remark?: string // Only used when updating user
  admin_permissions?: AdminPermissionMatrix
}

export type ManageUserAction =
  | 'promote'
  | 'demote'
  | 'enable'
  | 'disable'
  | 'delete'
  | 'add_quota'

export type QuotaAdjustMode = 'add' | 'subtract' | 'override'

export interface ManageUserQuotaPayload {
  id: number
  action: 'add_quota'
  mode: QuotaAdjustMode
  value: number
}

// ============================================================================
// Custom Pricing (千人千面) Types
// ============================================================================

export interface UserCustomPricingGroup {
  ratio: number
}

export interface UserCustomPricingPayload {
  enabled: boolean
  groups: Record<string, UserCustomPricingGroup>
  /** Groups made visible to this user on top of their normal group set */
  extra_groups?: Record<string, string>
  /** Groups hidden from this user regardless of the normal group set */
  hide_groups?: string[]
}

export interface UserCustomPricingGroupDetail {
  ratio: number | null
  default_ratio: number
  configured: boolean
}

export interface UserCustomPricingDetail {
  enabled: boolean
  groups: Record<string, UserCustomPricingGroupDetail>
  extra_groups?: Record<string, string> | null
  hide_groups?: string[] | null
  /** Every system group with its default ratio, used for visibility overrides */
  all_groups?: Record<string, number> | null
}

/** One group-ratio override shown on the custom-pricing list. */
export interface CustomPricingConfiguredGroup {
  name: string
  ratio: number
}

export interface CustomPricingUserItem {
  id: number
  username: string
  display_name: string
  group: string
  configured_groups: number
  total_groups: number
  missing_groups: string[] | null
  /** Only groups with an admin-configured override (name + ratio). */
  groups: CustomPricingConfiguredGroup[]
}

/** Admin view of one user's direct-reseller ownership. */
export interface UserResellerBinding {
  customer_id: number
  customer_username: string
  bound: boolean
  binding_id: number
  reseller_id: number
  reseller_username: string
  reseller_status: string
  registration_source: string
  bound_at: number
  current_multiplier_bps: number
  multiplier_source: string
  /** True when this user runs a reseller center of its own. */
  is_reseller: boolean
  own_customer_count: number
}

export interface BindUserToResellerPayload {
  reseller_id?: number
  reseller_username?: string
}

// ============================================================================
// Dialog Types
// ============================================================================

export type UsersDialogType = 'create' | 'update' | 'delete'
