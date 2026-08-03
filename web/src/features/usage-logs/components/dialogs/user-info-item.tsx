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
import { CopyButton } from '@/components/copy-button'
import { Label } from '@/components/ui/label'

interface UserInfoItemProps {
  label: string
  value: string | number
  copyable?: boolean
  copyTooltip?: string
  copiedTooltip?: string
  copyAriaLabel?: string
}

/** Label + value pair used across the usage-logs user detail dialog. */
export function UserInfoItem(props: UserInfoItemProps) {
  if (!props.copyable) {
    return (
      <div className='space-y-1.5'>
        <Label className='text-muted-foreground text-xs'>{props.label}</Label>
        <div className='text-sm font-semibold'>{props.value}</div>
      </div>
    )
  }

  return (
    <div className='space-y-1.5'>
      <Label className='text-muted-foreground text-xs'>{props.label}</Label>
      <div className='flex min-w-0 items-center gap-1'>
        <div
          className='text-sm font-semibold break-all select-all'
          title={String(props.value)}
        >
          {props.value}
        </div>
        <CopyButton
          value={String(props.value)}
          size='icon'
          className='size-7'
          iconClassName='size-3.5'
          tooltip={props.copyTooltip}
          successTooltip={props.copiedTooltip}
          aria-label={props.copyAriaLabel}
        />
      </div>
    </div>
  )
}
