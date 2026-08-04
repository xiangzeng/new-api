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

export interface ResellerEnvelope<T> {
  success: boolean
  data: T
  message: string
}

export interface ResellerPage<T> {
  page: number
  page_size: number
  total: number
  items: T[]
}

export interface ResellerStatus {
  enabled: boolean
  status?: 'active' | 'frozen'
  receive_public_id?: string
  pricing_version?: number
  pending_commission_quota: number
  available_commission_quota: number
  customer_count: number
  wallet_quota: number
  outbound_used_24h: number
  created_at?: number
}

export interface ResellerInvitation {
  path: string
  token: string
  expires_at: number
  version: number
}

export interface ResellerSecurityStatus {
  configured: boolean
  password_version: number
  password_updated_at: number
  outbound_frozen: boolean
  outbound_frozen_until: number
}

export interface ResellerPricingRule {
  id: number
  owner_type: 'default' | 'customer'
  owner_id: number
  group_name: string
  current_multiplier_bps: number
  pending_multiplier_bps: number
  pending_effective_at: number
  version: number
}

export interface ResellerPricingResponse {
  binding_id?: number
  customer_id?: number
  pricing_version: number
  rules: Record<string, ResellerPricingRule>
  multiplier_min_bps?: number
  multiplier_max_bps?: number
}

export interface ResellerCustomer {
  binding_id: number
  customer_id: number
  username: string
  display_name: string
  group: string
  quota: number
  used_quota: number
  registration_source: string
  bound_at: number
  pricing_version: number
}

export interface ResellerTransfer {
  public_id: string
  direction: 'sent' | 'received'
  counterparty_user_id: number
  counterparty_name: string
  amount: number
  quota: number
  created_at: number
}

export interface ResellerLedgerItem {
  id: number
  reference: string
  kind: string
  related_commission_id: number
  delta_quota: number
  amount_quota: number
  created_at: number
}

export interface ResellerVoucherBatch {
  id: number
  public_id: string
  count: number
  amount: number
  total_quota: number
  note: string
  created_at: number
}

export interface ResellerVoucher {
  id: number
  public_id: string
  batch_id: number
  amount: number
  quota: number
  redeemed_by: number
  redeemed_at: number
  created_at: number
}

export type ResellerSecurityScope =
  | 'reseller.security.password'
  | 'reseller.security.password_reset'
  | 'reseller.transfer'
  | 'reseller.commission.convert'
  | 'reseller.voucher.issue'
  | 'reseller.voucher.reveal'
  | 'reseller.receive_address.rotate'
