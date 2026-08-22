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
import { createFileRoute, redirect } from '@tanstack/react-router'
import z from 'zod'

import { UserDetail } from '@/features/users/components/detail/user-detail'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

const userDetailSearchSchema = z.object({
  /** Unix seconds, matching the `/api/data/flow` contract. */
  start: z.number().optional().catch(undefined),
  end: z.number().optional().catch(undefined),
  dim: z
    .enum(['group', 'model', 'channel', 'token', 'node'])
    .optional()
    .catch(undefined),
  group: z.string().optional().catch(undefined),
  model: z.string().optional().catch(undefined),
  channel: z.string().optional().catch(undefined),
  token: z.string().optional().catch(undefined),
  node: z.string().optional().catch(undefined),
  q: z.string().optional().catch(undefined),
})

export const Route = createFileRoute('/_authenticated/users/$userId')({
  beforeLoad: ({ params }) => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({
        to: '/403',
      })
    }

    const userId = Number(params.userId)
    if (!Number.isInteger(userId) || userId <= 0) {
      throw redirect({
        to: '/users',
      })
    }
  },
  validateSearch: userDetailSearchSchema,
  component: UserDetail,
})
