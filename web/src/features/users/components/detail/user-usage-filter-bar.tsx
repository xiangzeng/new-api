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
import { RotateCcw, Search } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'

import { USER_USAGE_DIMENSION_META } from '../../constants'
import {
  USER_USAGE_UNKNOWN_KEY,
  getUserUsageMonthOptions,
  getUserUsageMonthRange,
  matchUserUsageMonth,
  type UserUsageDimension,
  type UserUsageDimensionFilters,
  type UserUsageDimensionValue,
  type UserUsageRange,
} from '../../lib/user-usage'

/** Sentinel select values; Base UI selects need a non-empty value per item. */
const ALL_VALUE = '__all__'
const CUSTOM_RANGE_VALUE = '__custom__'
const MONTH_OPTION_COUNT = 12

interface UserUsageFilterBarProps {
  range: UserUsageRange
  onRangeChange: (range: UserUsageRange) => void
  dimensions: UserUsageDimension[]
  options: Partial<Record<UserUsageDimension, UserUsageDimensionValue[]>>
  filters: UserUsageDimensionFilters
  onFilterChange: (dimension: UserUsageDimension, value: string) => void
  keyword: string
  onKeywordChange: (keyword: string) => void
  onReset: () => void
  hasActiveFilters: boolean
}

function DimensionFilter(props: {
  dimension: UserUsageDimension
  options: UserUsageDimensionValue[]
  value?: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const meta = USER_USAGE_DIMENSION_META[props.dimension]
  const allLabel = t(meta.allLabelKey)
  const items = useMemo(
    () => [
      { value: ALL_VALUE, label: allLabel },
      ...props.options.map((option) => ({
        value: option.key,
        label:
          option.key === USER_USAGE_UNKNOWN_KEY ? t('Unknown') : option.name,
      })),
    ],
    [allLabel, props.options, t]
  )
  const value = props.value || ALL_VALUE
  const label = items.find((item) => item.value === value)?.label ?? allLabel

  return (
    <Select
      items={items}
      value={value}
      onValueChange={(next) => {
        props.onChange(next === ALL_VALUE || next === null ? '' : String(next))
      }}
    >
      <SelectTrigger
        size='sm'
        className='w-full min-w-0 sm:w-44'
        aria-label={t(meta.labelKey)}
      >
        <SelectValue>{label}</SelectValue>
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false}>
        <SelectGroup>
          {items.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.label}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  )
}

export function UserUsageFilterBar(props: UserUsageFilterBarProps) {
  const { t } = useTranslation()
  const selectedMonth = matchUserUsageMonth(props.range) ?? CUSTOM_RANGE_VALUE
  const monthItems = useMemo(() => {
    const months = getUserUsageMonthOptions(MONTH_OPTION_COUNT)
    if (
      selectedMonth !== CUSTOM_RANGE_VALUE &&
      !months.includes(selectedMonth)
    ) {
      months.unshift(selectedMonth)
    }
    return [
      { value: CUSTOM_RANGE_VALUE, label: t('Custom range') },
      ...months.map((month) => ({ value: month, label: month })),
    ]
  }, [selectedMonth, t])
  const monthLabel =
    monthItems.find((item) => item.value === selectedMonth)?.label ??
    t('Custom range')

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <Select
        items={monthItems}
        value={selectedMonth}
        onValueChange={(next) => {
          if (next === null || next === CUSTOM_RANGE_VALUE) return
          const range = getUserUsageMonthRange(String(next))
          if (range) props.onRangeChange(range)
        }}
      >
        <SelectTrigger
          size='sm'
          className='w-full min-w-0 sm:w-36'
          aria-label={t('Select month')}
        >
          <SelectValue>{monthLabel}</SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {monthItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>

      <CompactDateTimeRangePicker
        start={new Date(props.range.start * 1000)}
        end={new Date(props.range.end * 1000)}
        onChange={(next) => {
          if (!next.start || !next.end) return
          props.onRangeChange({
            start: Math.floor(next.start.getTime() / 1000),
            end: Math.floor(next.end.getTime() / 1000),
          })
        }}
        className='h-8 w-full sm:w-[19rem]'
      />

      {props.dimensions.map((dimension) => (
        <DimensionFilter
          key={dimension}
          dimension={dimension}
          options={props.options[dimension] ?? []}
          value={props.filters[dimension]}
          onChange={(value) => props.onFilterChange(dimension, value)}
        />
      ))}

      <div className='relative w-full sm:w-48'>
        <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2' />
        <Input
          value={props.keyword}
          onChange={(event) => props.onKeywordChange(event.target.value)}
          placeholder={t('Filter results')}
          aria-label={t('Filter results')}
          className='h-8 pl-8 text-sm'
        />
      </div>

      {props.hasActiveFilters && (
        <Button
          type='button'
          variant='ghost'
          size='sm'
          className='h-8'
          onClick={props.onReset}
        >
          <RotateCcw className='size-3.5' />
          {t('Reset filters')}
        </Button>
      )}
    </div>
  )
}
