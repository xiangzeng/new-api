/*
Copyright (C) 2025 QuantumNous

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

import React from 'react';
import { Space, Tag, Tooltip, Typography } from '@douyinfe/semi-ui';
import { renderGroup, renderNumber, renderQuota } from '../../../helpers';

const { Text } = Typography;

const renderUserCell = ({ id, username, displayName, email, isDeleted, t }) => (
  <div className='flex flex-col gap-1 min-w-[180px]'>
    <Space spacing={4} wrap>
      <Text strong copyable={{ content: username || '' }}>
        {username || '-'}
      </Text>
      <Tag size='small' color='white' shape='circle'>
        ID {id}
      </Tag>
      {isDeleted && (
        <Tag size='small' color='red' shape='circle'>
          {t('已注销')}
        </Tag>
      )}
    </Space>
    <div className='text-xs text-gray-500 break-all'>
      {displayName && displayName !== username ? displayName : email || '-'}
    </div>
  </div>
);

const renderStatus = (record, t) => {
  if (record.is_deleted || record.inviter_deleted) {
    return (
      <Tag color='red' shape='circle'>
        {t('已注销')}
      </Tag>
    );
  }
  if (record.status === 2) {
    return (
      <Tag color='red' shape='circle'>
        {t('已禁用')}
      </Tag>
    );
  }
  return (
    <Tag color='green' shape='circle'>
      {t('已启用')}
    </Tag>
  );
};

const renderQuotaPair = (
  primary,
  secondary,
  primaryLabel,
  secondaryLabel,
  t,
) => (
  <Tooltip
    content={
      <div className='text-xs'>
        <div>
          {t(primaryLabel)}: {renderQuota(primary || 0)}
        </div>
        <div>
          {t(secondaryLabel)}: {renderQuota(secondary || 0)}
        </div>
      </div>
    }
  >
    <div className='flex flex-col items-start gap-1'>
      <Text strong>{renderQuota(primary || 0)}</Text>
      <span className='text-xs text-gray-500'>
        {renderQuota(secondary || 0)}
      </span>
    </div>
  </Tooltip>
);

const renderPeriodStats = (record, t) => (
  <Tooltip
    content={
      <div className='text-xs'>
        <div>
          {t('筛选消耗')}: {renderQuota(record.period_quota || 0)}
        </div>
        <div>
          {t('筛选请求')}: {renderNumber(record.period_request_count || 0)}
        </div>
        <div>
          Prompt Tokens: {renderNumber(record.period_prompt_tokens || 0)}
        </div>
        <div>
          Completion Tokens:{' '}
          {renderNumber(record.period_completion_tokens || 0)}
        </div>
      </div>
    }
  >
    <div className='flex flex-col items-start gap-1'>
      <Text strong>{renderQuota(record.period_quota || 0)}</Text>
      <span className='text-xs text-gray-500'>
        {renderNumber(record.period_request_count || 0)} {t('次请求')}
      </span>
    </div>
  </Tooltip>
);

export const getInvitationSummaryColumns = ({ t }) => [
  {
    title: t('邀请人'),
    dataIndex: 'inviter_username',
    key: 'inviter',
    fixed: 'left',
    width: 240,
    render: (text, record) =>
      renderUserCell({
        id: record.inviter_id,
        username: text,
        displayName: record.inviter_display_name,
        email: record.inviter_email,
        isDeleted: record.inviter_deleted,
        t,
      }),
  },
  {
    title: t('邀请码'),
    dataIndex: 'aff_code',
    key: 'aff_code',
    width: 120,
    render: (text) =>
      text ? (
        <Text copyable={{ content: text }} className='font-mono'>
          {text}
        </Text>
      ) : (
        '-'
      ),
  },
  {
    title: t('邀请人数'),
    dataIndex: 'invitee_count',
    key: 'invitee_count',
    width: 110,
    render: (text) => (
      <Tag color='blue' shape='circle'>
        {renderNumber(text || 0)}
      </Tag>
    ),
  },
  {
    title: t('被邀请人累计消耗'),
    dataIndex: 'invitee_total_used_quota',
    key: 'invitee_total_used_quota',
    width: 170,
    render: (text, record) => (
      <Tooltip
        content={`${t('累计请求')}: ${renderNumber(
          record.invitee_total_request_count || 0,
        )}`}
      >
        <div className='flex flex-col items-start gap-1'>
          <Text strong>{renderQuota(text || 0)}</Text>
          <span className='text-xs text-gray-500'>
            {renderNumber(record.invitee_total_request_count || 0)}{' '}
            {t('次请求')}
          </span>
        </div>
      </Tooltip>
    ),
  },
  {
    title: t('筛选范围消耗'),
    dataIndex: 'period_quota',
    key: 'period_quota',
    width: 160,
    render: (text, record) => renderPeriodStats(record, t),
  },
  {
    title: t('邀请奖励'),
    dataIndex: 'aff_history_quota',
    key: 'aff_history_quota',
    width: 160,
    render: (text, record) =>
      renderQuotaPair(text, record.aff_quota, '历史奖励', '可转余额', t),
  },
];

export const getInvitationInviteeColumns = ({ t }) => [
  {
    title: t('被邀请人'),
    dataIndex: 'username',
    key: 'username',
    width: 220,
    render: (text, record) =>
      renderUserCell({
        id: record.invitee_id,
        username: text,
        displayName: record.display_name,
        email: record.email,
        isDeleted: record.is_deleted,
        t,
      }),
  },
  {
    title: t('状态'),
    dataIndex: 'status',
    key: 'status',
    width: 100,
    render: (text, record) => renderStatus(record, t),
  },
  {
    title: t('分组'),
    dataIndex: 'group',
    key: 'group',
    width: 120,
    render: (text) => renderGroup(text || ''),
  },
  {
    title: t('剩余额度'),
    dataIndex: 'quota',
    key: 'quota',
    width: 130,
    render: (text) => renderQuota(text || 0),
  },
  {
    title: t('累计消耗'),
    dataIndex: 'used_quota',
    key: 'used_quota',
    width: 150,
    render: (text, record) => (
      <Tooltip
        content={`${t('累计请求')}: ${renderNumber(record.request_count || 0)}`}
      >
        <div className='flex flex-col items-start gap-1'>
          <Text strong>{renderQuota(text || 0)}</Text>
          <span className='text-xs text-gray-500'>
            {renderNumber(record.request_count || 0)} {t('次请求')}
          </span>
        </div>
      </Tooltip>
    ),
  },
  {
    title: t('筛选范围消耗'),
    dataIndex: 'period_quota',
    key: 'period_quota',
    width: 150,
    render: (text, record) => renderPeriodStats(record, t),
  },
];
