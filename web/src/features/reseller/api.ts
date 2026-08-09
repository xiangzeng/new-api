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
  ResellerCustomer,
  ResellerEnvelope,
  ResellerInvitation,
  ResellerLedgerItem,
  ResellerPage,
  ResellerPricingResponse,
  ResellerSecurityStatus,
  ResellerStatus,
  ResellerTransfer,
  ResellerTransferPreview,
  ResellerVoucher,
  ResellerVoucherBatch,
} from './types'

const sensitiveConfig = (proof?: string, idempotencyKey?: string) => ({
  headers: {
    ...(proof ? { 'X-Security-Proof': proof } : {}),
    ...(idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : {}),
  },
  skipErrorHandler: true,
})

export const newIdempotencyKey = () =>
  typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `reseller-${Date.now()}-${Math.random().toString(16).slice(2)}`

export async function getResellerStatus() {
  const response = await api.get<ResellerEnvelope<ResellerStatus>>(
    '/api/reseller/status',
    { skipErrorHandler: true }
  )
  return response.data.data
}

export async function enableResellerProfile() {
  const response = await api.post<ResellerEnvelope<unknown>>(
    '/api/reseller/profile',
    {},
    { skipErrorHandler: true }
  )
  return response.data
}

export async function getResellerInvitation() {
  const response = await api.get<ResellerEnvelope<ResellerInvitation>>(
    '/api/reseller/invitation'
  )
  return response.data.data
}

export async function getResellerSecurity() {
  const response = await api.get<ResellerEnvelope<ResellerSecurityStatus>>(
    '/api/reseller/security'
  )
  return response.data.data
}

export async function getDefaultPricing() {
  const response = await api.get<ResellerEnvelope<ResellerPricingResponse>>(
    '/api/reseller/pricing/default'
  )
  return response.data.data
}

export async function getCustomerPricing(bindingId: number) {
  const response = await api.get<ResellerEnvelope<ResellerPricingResponse>>(
    `/api/reseller/customers/${bindingId}/pricing`
  )
  return response.data.data
}

export async function updatePricing(
  owner: 'default' | number,
  payload: {
    multiplier_bps: number
    group_multipliers_bps: Record<string, number>
    expected_version: number
    quota_password: string
  }
) {
  const path =
    owner === 'default'
      ? '/api/reseller/pricing/default'
      : `/api/reseller/customers/${owner}/pricing`
  const response = await api.put<ResellerEnvelope<ResellerPricingResponse>>(
    path,
    payload,
    { skipErrorHandler: true }
  )
  return response.data
}

export async function deletePricing(
  owner: 'default' | number,
  payload: { group_name: string; expected_version: number }
) {
  const path =
    owner === 'default'
      ? '/api/reseller/pricing/default'
      : `/api/reseller/customers/${owner}/pricing`
  const response = await api.delete<
    ResellerEnvelope<{ pricing_version: number }>
  >(path, { data: payload, skipErrorHandler: true })
  return response.data
}

type ResellerPagePayload<T> = Omit<ResellerPage<T>, 'items'> & {
  items?: T[] | null
}

export function normalizeResellerPage<T>(
  page: ResellerPagePayload<T>
): ResellerPage<T> {
  return {
    ...page,
    items: Array.isArray(page.items) ? page.items : [],
  }
}

async function getPage<T>(path: string, page = 1, pageSize = 50) {
  const response = await api.get<ResellerEnvelope<ResellerPagePayload<T>>>(
    path,
    {
      params: { p: page, page_size: pageSize },
    }
  )
  return normalizeResellerPage(response.data.data)
}

export const getResellerCustomers = (page = 1, pageSize = 20) =>
  getPage<ResellerCustomer>('/api/reseller/customers', page, pageSize)
export const getResellerTransfers = (page = 1) =>
  getPage<ResellerTransfer>('/api/reseller/transfers', page)
export const getResellerLedger = (page = 1) =>
  getPage<ResellerLedgerItem>('/api/reseller/ledger', page)
export const getResellerVouchers = (page = 1) =>
  getPage<ResellerVoucher>('/api/reseller/vouchers', page)
export const getResellerVouchersByStatus = (
  page = 1,
  status: 'all' | 'pending' | 'used' = 'all'
) =>
  getPage<ResellerVoucher>(
    `/api/reseller/vouchers${status === 'all' ? '' : `?status=${status}`}`,
    page
  )
export const getResellerVoucherBatches = (page = 1) =>
  getPage<ResellerVoucherBatch>('/api/reseller/vouchers/batches', page)

export async function setQuotaPassword(
  quotaPassword: string,
  loginPassword: string,
  proof?: string
) {
  return api.post(
    '/api/reseller/security/password',
    { quota_password: quotaPassword, login_password: loginPassword },
    sensitiveConfig(proof)
  )
}

export async function changeQuotaPassword(
  currentPassword: string,
  newPassword: string
) {
  return api.put(
    '/api/reseller/security/password',
    {
      current_quota_password: currentPassword,
      new_quota_password: newPassword,
    },
    sensitiveConfig()
  )
}

export async function resetQuotaPassword(
  newPassword: string,
  loginPassword: string,
  proof?: string
) {
  return api.post(
    '/api/reseller/security/password/reset',
    { quota_password: newPassword, login_password: loginPassword },
    sensitiveConfig(proof)
  )
}

export async function updateCustomerNote(bindingId: number, note: string) {
  const response = await api.put<ResellerEnvelope<{ note: string }>>(
    `/api/reseller/customers/${bindingId}/note`,
    { note },
    { skipErrorHandler: true }
  )
  return response.data
}

export async function previewTransfer(bindingId: number, quota: number) {
  return api.post(
    '/api/reseller/transfers/preview',
    { binding_id: bindingId, quota: String(Math.trunc(quota)) },
    sensitiveConfig()
  )
}

export async function commitTransfer(
  preview: ResellerTransferPreview,
  password: string,
  idempotencyKey: string
) {
  return api.post(
    '/api/reseller/transfers/commit',
    {
      recipient_user_id: preview.recipient_user_id,
      recipient_username: preview.recipient_username,
      quota: String(Math.trunc(preview.quota)),
      nonce: preview.nonce,
      quota_password: password,
    },
    sensitiveConfig(undefined, idempotencyKey)
  )
}

export async function convertCommission(
  quota: number,
  password: string,
  idempotencyKey: string
) {
  return api.post(
    '/api/reseller/commission/convert',
    { quota: String(Math.trunc(quota)), quota_password: password },
    sensitiveConfig(undefined, idempotencyKey)
  )
}

export async function issueVouchers(
  count: number,
  amount: number,
  note: string,
  password: string,
  idempotencyKey: string
) {
  return api.post(
    count === 1 ? '/api/reseller/vouchers' : '/api/reseller/vouchers/batch',
    { count, amount, note, quota_password: password },
    sensitiveConfig(undefined, idempotencyKey)
  )
}

export async function revealVoucher(publicId: string, password: string) {
  return api.post(
    `/api/reseller/vouchers/${publicId}/reveal`,
    { quota_password: password },
    sensitiveConfig()
  )
}

export async function revealVoucherBatch(publicId: string, password: string) {
  return api.post(
    `/api/reseller/vouchers/batch/${publicId}/reveal`,
    { quota_password: password },
    sensitiveConfig()
  )
}
