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

import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import { useTableCompactMode } from '../common/useTableCompactMode';

const toUnixTimestamp = (value) => {
  if (!value) return 0;
  const parsed = Date.parse(value);
  if (!Number.isFinite(parsed)) return 0;
  return Math.floor(parsed / 1000);
};

export const useInvitationsData = () => {
  const { t } = useTranslation();
  const [compactMode, setCompactMode] = useTableCompactMode('invitations');

  const [summaries, setSummaries] = useState([]);
  const [loading, setLoading] = useState(true);
  const [searching, setSearching] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [pageSize, setPageSize] = useState(ITEMS_PER_PAGE);
  const [summaryCount, setSummaryCount] = useState(0);
  const [formApi, setFormApi] = useState(null);
  const [periodParams, setPeriodParams] = useState({
    start_timestamp: 0,
    end_timestamp: 0,
  });

  const formInitValues = {
    keyword: '',
    dateRange: [],
  };

  const getFormValues = () => {
    const formValues = formApi ? formApi.getValues() : {};
    const dateRange = Array.isArray(formValues.dateRange)
      ? formValues.dateRange
      : [];
    return {
      keyword: formValues.keyword || '',
      start_timestamp: toUnixTimestamp(dateRange[0]),
      end_timestamp: toUnixTimestamp(dateRange[1]),
    };
  };

  const setSummaryFormat = (items) => {
    setSummaries(
      (items || []).map((item) => ({
        ...item,
        key: item.inviter_id,
      })),
    );
  };

  const loadSummaries = async (page = 1, size = pageSize) => {
    setLoading(true);
    try {
      const filters = getFormValues();
      setPeriodParams({
        start_timestamp: filters.start_timestamp,
        end_timestamp: filters.end_timestamp,
      });
      const params = new URLSearchParams({
        p: String(page),
        page_size: String(size),
        keyword: filters.keyword,
        start_timestamp: String(filters.start_timestamp),
        end_timestamp: String(filters.end_timestamp),
      });
      const res = await API.get(`/api/invitation/summary?${params.toString()}`);
      const { success, message, data } = res.data;
      if (success) {
        setActivePage(data.page <= 0 ? 1 : data.page);
        setPageSize(data.page_size || size);
        setSummaryCount(data.total || 0);
        setSummaryFormat(data.items || []);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };

  const searchSummaries = async () => {
    setSearching(true);
    await loadSummaries(1, pageSize);
    setSearching(false);
  };

  const handlePageChange = (page) => {
    setActivePage(page);
    loadSummaries(page, pageSize);
  };

  const handlePageSizeChange = (size) => {
    setPageSize(size);
    setActivePage(1);
    loadSummaries(1, size);
  };

  const periodKey = useMemo(
    () => `${periodParams.start_timestamp}-${periodParams.end_timestamp}`,
    [periodParams],
  );

  useEffect(() => {
    loadSummaries(1, pageSize);
  }, []);

  return {
    summaries,
    loading,
    searching,
    activePage,
    pageSize,
    summaryCount,
    compactMode,
    setCompactMode,
    formInitValues,
    setFormApi,
    loadSummaries,
    searchSummaries,
    handlePageChange,
    handlePageSizeChange,
    periodParams,
    periodKey,
    t,
  };
};

export default useInvitationsData;
