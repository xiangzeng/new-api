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
  OpenApp,
  OpenAppApiResponse,
  OpenAppRequest,
  OpenAppSecretPayload,
} from './types'

export async function getOpenApps(): Promise<OpenAppApiResponse<OpenApp[]>> {
  const res = await api.get('/api/open-app/')
  return res.data
}

/** Creates a partner application and returns its one-time clear-text secret. */
export async function createOpenApp(
  data: OpenAppRequest
): Promise<OpenAppApiResponse<OpenAppSecretPayload>> {
  const res = await api.post('/api/open-app/', data)
  return res.data
}

export async function updateOpenApp(
  id: number,
  data: OpenAppRequest
): Promise<OpenAppApiResponse<OpenApp>> {
  const res = await api.put(`/api/open-app/${id}`, data)
  return res.data
}

/**
 * Rotates the secret. Every credential issued under the previous secret is
 * revoked server side, so partners must re-authorize their users afterwards.
 */
export async function resetOpenAppSecret(
  id: number
): Promise<OpenAppApiResponse<OpenAppSecretPayload>> {
  const res = await api.post(`/api/open-app/${id}/reset-secret`)
  return res.data
}

export async function deleteOpenApp(
  id: number
): Promise<OpenAppApiResponse<null>> {
  const res = await api.delete(`/api/open-app/${id}`)
  return res.data
}
