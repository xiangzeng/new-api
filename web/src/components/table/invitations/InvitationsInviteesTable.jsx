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

import React, { useEffect, useMemo, useState } from 'react';
import { Empty } from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { API, showError } from '../../../helpers';
import CardTable from '../../common/ui/CardTable';
import { getInvitationInviteeColumns } from './InvitationsColumnDefs';

const InvitationsInviteesTable = ({
  inviterId,
  periodParams,
  periodKey,
  t,
}) => {
  const [invitees, setInvitees] = useState([]);
  const [loading, setLoading] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [total, setTotal] = useState(0);

  const columns = useMemo(() => getInvitationInviteeColumns({ t }), [t]);

  const loadInvitees = async (page = activePage, size = pageSize) => {
    if (!inviterId) return;
    setLoading(true);
    try {
      const params = new URLSearchParams({
        inviter_id: String(inviterId),
        p: String(page),
        page_size: String(size),
        start_timestamp: String(periodParams?.start_timestamp || 0),
        end_timestamp: String(periodParams?.end_timestamp || 0),
      });
      const res = await API.get(
        `/api/invitation/invitees?${params.toString()}`,
      );
      const { success, message, data } = res.data;
      if (success) {
        setInvitees(
          (data.items || []).map((item) => ({
            ...item,
            key: item.invitee_id,
          })),
        );
        setActivePage(data.page <= 0 ? 1 : data.page);
        setPageSize(data.page_size || size);
        setTotal(data.total || 0);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    setActivePage(1);
    loadInvitees(1, pageSize);
  }, [inviterId, periodKey]);

  return (
    <div
      className='py-3 px-2'
      style={{ background: 'var(--semi-color-fill-0)' }}
    >
      <CardTable
        columns={columns}
        dataSource={invitees}
        rowKey='invitee_id'
        loading={loading}
        scroll={{ x: 'max-content' }}
        pagination={{
          currentPage: activePage,
          pageSize,
          total,
          showSizeChanger: true,
          pageSizeOptions: [10, 20, 50, 100],
          onPageSizeChange: (size) => {
            setPageSize(size);
            setActivePage(1);
            loadInvitees(1, size);
          },
          onPageChange: (page) => {
            setActivePage(page);
            loadInvitees(page, pageSize);
          },
        }}
        size='small'
        empty={
          <Empty
            image={<IllustrationNoResult style={{ width: 120, height: 120 }} />}
            darkModeImage={
              <IllustrationNoResultDark style={{ width: 120, height: 120 }} />
            }
            description={t('暂无被邀请人')}
            style={{ padding: 20 }}
          />
        }
      />
    </div>
  );
};

export default InvitationsInviteesTable;
