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
import CardPro from '../../common/ui/CardPro';
import InvitationsDescription from './InvitationsDescription';
import InvitationsFilters from './InvitationsFilters';
import InvitationsTable from './InvitationsTable';
import { useInvitationsData } from '../../../hooks/invitations/useInvitationsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';

const InvitationsPage = () => {
  const invitationsData = useInvitationsData();
  const isMobile = useIsMobile();

  const {
    compactMode,
    setCompactMode,
    formInitValues,
    setFormApi,
    searchSummaries,
    loadSummaries,
    pageSize,
    loading,
    searching,
    t,
  } = invitationsData;

  return (
    <CardPro
      type='type1'
      descriptionArea={
        <InvitationsDescription
          compactMode={compactMode}
          setCompactMode={setCompactMode}
          t={t}
        />
      }
      actionsArea={
        <div className='flex flex-col md:flex-row justify-end items-center gap-2 w-full'>
          <InvitationsFilters
            formInitValues={formInitValues}
            setFormApi={setFormApi}
            searchSummaries={searchSummaries}
            loadSummaries={loadSummaries}
            pageSize={pageSize}
            loading={loading}
            searching={searching}
            t={t}
          />
        </div>
      }
      paginationArea={createCardProPagination({
        currentPage: invitationsData.activePage,
        pageSize: invitationsData.pageSize,
        total: invitationsData.summaryCount,
        onPageChange: invitationsData.handlePageChange,
        onPageSizeChange: invitationsData.handlePageSizeChange,
        isMobile: isMobile,
        t: invitationsData.t,
      })}
      t={invitationsData.t}
    >
      <InvitationsTable {...invitationsData} />
    </CardPro>
  );
};

export default InvitationsPage;
