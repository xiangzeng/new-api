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
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'

interface CustomPricingVisibilitySectionProps {
  allGroups: string[]
  extraGroups: Record<string, string>
  hideGroups: string[]
  onExtraGroupsChange: (extraGroups: Record<string, string>) => void
  onHideGroupsChange: (hideGroups: string[]) => void
  disabled: boolean
}

export function CustomPricingVisibilitySection(
  props: CustomPricingVisibilitySectionProps
) {
  const { t } = useTranslation()

  const toggleExtraGroup = (name: string, checked: boolean) => {
    const next = { ...props.extraGroups }
    if (checked) {
      next[name] = name
    } else {
      delete next[name]
    }
    props.onExtraGroupsChange(next)
  }

  const toggleHideGroup = (name: string, checked: boolean) => {
    const next = checked
      ? [...props.hideGroups, name]
      : props.hideGroups.filter((group) => group !== name)
    props.onHideGroupsChange(next)
  }

  if (props.allGroups.length === 0) {
    return null
  }

  return (
    <div className='border-border space-y-3 rounded-lg border p-3'>
      <div className='space-y-1'>
        <p className='text-sm font-medium'>{t('Group visibility overrides')}</p>
        <p className='text-muted-foreground text-xs'>
          {t(
            'Open or hide specific groups for this user only. Extra groups become selectable even when they are not part of the user group set.'
          )}
        </p>
      </div>

      <div className='space-y-2'>
        <p className='text-sm font-medium'>{t('Extra visible groups')}</p>
        <div className='flex flex-wrap gap-x-4 gap-y-2'>
          {props.allGroups.map((name) => (
            <label key={name} className='flex items-center gap-2'>
              <Checkbox
                checked={props.extraGroups[name] !== undefined}
                onCheckedChange={(checked) =>
                  toggleExtraGroup(name, checked === true)
                }
                disabled={props.disabled}
              />
              <span className='text-sm'>{name}</span>
            </label>
          ))}
        </div>
      </div>

      <div className='space-y-2'>
        <p className='text-sm font-medium'>{t('Force hidden groups')}</p>
        <div className='flex flex-wrap gap-x-4 gap-y-2'>
          {props.allGroups.map((name) => (
            <label key={name} className='flex items-center gap-2'>
              <Checkbox
                checked={props.hideGroups.includes(name)}
                onCheckedChange={(checked) =>
                  toggleHideGroup(name, checked === true)
                }
                disabled={props.disabled}
              />
              <span className='text-sm'>{name}</span>
            </label>
          ))}
        </div>
      </div>
    </div>
  )
}
