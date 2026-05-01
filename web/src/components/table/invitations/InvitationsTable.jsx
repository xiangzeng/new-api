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

import React, { useMemo } from 'react';
import { Empty } from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import CardTable from '../../common/ui/CardTable';
import { getInvitationSummaryColumns } from './InvitationsColumnDefs';
import InvitationsInviteesTable from './InvitationsInviteesTable';

const InvitationsTable = (invitationsData) => {
  const { summaries, loading, compactMode, periodParams, periodKey, t } =
    invitationsData;

  const columns = useMemo(() => getInvitationSummaryColumns({ t }), [t]);

  const tableColumns = useMemo(() => {
    return compactMode
      ? columns.map((column) => {
          const { fixed, ...rest } = column;
          return rest;
        })
      : columns;
  }, [compactMode, columns]);

  const hasExpandableRows = () =>
    summaries.some((summary) => Number(summary.invitee_count || 0) > 0);

  const expandRowRender = (record) => (
    <InvitationsInviteesTable
      inviterId={record.inviter_id}
      periodParams={periodParams}
      periodKey={periodKey}
      t={t}
    />
  );

  return (
    <CardTable
      columns={tableColumns}
      dataSource={summaries}
      rowKey='inviter_id'
      loading={loading}
      scroll={compactMode ? undefined : { x: 'max-content' }}
      {...(hasExpandableRows() && {
        expandedRowRender: expandRowRender,
        expandRowByClick: true,
        rowExpandable: (record) => Number(record.invitee_count || 0) > 0,
      })}
      hidePagination={true}
      className='rounded-xl overflow-hidden'
      size='middle'
      empty={
        <Empty
          image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
          darkModeImage={
            <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
          }
          description={t('暂无邀请关系')}
          style={{ padding: 30 }}
        />
      }
    />
  );
};

export default InvitationsTable;
