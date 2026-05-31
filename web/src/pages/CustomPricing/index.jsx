import React, { useEffect, useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Card,
  Table,
  Button,
  Tag,
  Badge,
  Space,
  Typography,
  Modal,
  InputNumber,
  Switch,
  Select,
  Spin,
  Empty,
  Banner,
  SideSheet,
} from '@douyinfe/semi-ui';
import {
  IconPlus,
  IconDelete,
  IconEdit,
  IconSearch,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess } from '../../helpers';

const { Title, Text } = Typography;

const CustomPricing = () => {
  const { t } = useTranslation();
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [detailVisible, setDetailVisible] = useState(false);
  const [currentUser, setCurrentUser] = useState(null);
  const [detailData, setDetailData] = useState(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [addModalVisible, setAddModalVisible] = useState(false);
  const [searchUsers, setSearchUsers] = useState([]);
  const [searchLoading, setSearchLoading] = useState(false);
  const [selectedUserId, setSelectedUserId] = useState(null);

  const fetchUsers = useCallback(async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/user/custom-pricing/list');
      if (res.data.success) {
        setUsers(res.data.data || []);
      } else {
        showError(res.data.message);
      }
    } catch (e) {
      showError(e.message);
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  const openDetail = async (user) => {
    setCurrentUser(user);
    setDetailVisible(true);
    setDetailLoading(true);
    try {
      const res = await API.get(`/api/user/${user.id}/custom-pricing`);
      if (res.data.success) {
        setDetailData(res.data.data);
      } else {
        showError(res.data.message);
      }
    } catch (e) {
      showError(e.message);
    }
    setDetailLoading(false);
  };

  const saveDetail = async () => {
    if (!detailData || !currentUser) return;
    setSaving(true);
    try {
      const payload = {
        enabled: true,
        groups: {},
      };
      for (const [groupName, info] of Object.entries(detailData.groups)) {
        if (info.ratio !== null && info.ratio !== undefined && info.configured) {
          payload.groups[groupName] = { ratio: info.ratio };
        }
      }
      const res = await API.put(
        `/api/user/${currentUser.id}/custom-pricing`,
        payload,
      );
      if (res.data.success) {
        showSuccess(t('保存成功'));
        fetchUsers();
      } else {
        showError(res.data.message);
      }
    } catch (e) {
      showError(e.message);
    }
    setSaving(false);
  };

  const removeUser = async (userId) => {
    try {
      const res = await API.delete(`/api/user/${userId}/custom-pricing`);
      if (res.data.success) {
        showSuccess(t('已关闭千人千面'));
        fetchUsers();
      } else {
        showError(res.data.message);
      }
    } catch (e) {
      showError(e.message);
    }
  };

  const handleSearchUser = async (value) => {
    if (!value || value.length < 1) {
      setSearchUsers([]);
      return;
    }
    setSearchLoading(true);
    try {
      const res = await API.get(`/api/user/search?keyword=${value}`);
      if (res.data.success) {
        setSearchUsers(
          (res.data.data || []).map((u) => ({
            label: `${u.username} (${u.display_name || u.username})`,
            value: u.id,
          })),
        );
      }
    } catch (e) {
      showError(e.message);
    }
    setSearchLoading(false);
  };

  const addUserCustomPricing = async () => {
    if (!selectedUserId) return;
    try {
      const res = await API.put(`/api/user/${selectedUserId}/custom-pricing`, {
        enabled: true,
        groups: {},
      });
      if (res.data.success) {
        showSuccess(t('已开启千人千面'));
        setAddModalVisible(false);
        setSelectedUserId(null);
        fetchUsers();
      } else {
        showError(res.data.message);
      }
    } catch (e) {
      showError(e.message);
    }
  };

  const updateGroupRatio = (groupName, ratio) => {
    setDetailData((prev) => ({
      ...prev,
      groups: {
        ...prev.groups,
        [groupName]: {
          ...prev.groups[groupName],
          ratio: ratio,
          configured: true,
        },
      },
    }));
  };

  const toggleGroupConfigured = (groupName, configured) => {
    setDetailData((prev) => ({
      ...prev,
      groups: {
        ...prev.groups,
        [groupName]: {
          ...prev.groups[groupName],
          configured: configured,
          ratio: configured
            ? prev.groups[groupName].ratio ?? prev.groups[groupName].default_ratio
            : null,
        },
      },
    }));
  };

  const applyAllDefaults = () => {
    setDetailData((prev) => {
      const newGroups = { ...prev.groups };
      for (const [name, info] of Object.entries(newGroups)) {
        newGroups[name] = {
          ...info,
          configured: true,
          ratio: info.ratio ?? info.default_ratio,
        };
      }
      return { ...prev, groups: newGroups };
    });
  };

  const userColumns = [
    {
      title: t('用户名'),
      dataIndex: 'username',
      key: 'username',
      render: (text, record) => (
        <Text strong>{record.display_name || text}</Text>
      ),
    },
    {
      title: t('所属分组'),
      dataIndex: 'group',
      key: 'group',
      render: (text) => <Tag>{text}</Tag>,
    },
    {
      title: t('配置状态'),
      key: 'status',
      render: (_, record) => {
        const hasMissing =
          record.missing_groups && record.missing_groups.length > 0;
        return (
          <Space>
            <Text>
              {record.configured_groups}/{record.total_groups}
            </Text>
            {hasMissing && (
              <Badge count={record.missing_groups.length} type='danger' />
            )}
          </Space>
        );
      },
    },
    {
      title: t('操作'),
      key: 'action',
      render: (_, record) => (
        <Space>
          <Button
            theme='light'
            type='primary'
            icon={<IconEdit />}
            size='small'
            onClick={() => openDetail(record)}
          >
            {t('编辑')}
          </Button>
          <Button
            theme='light'
            type='danger'
            icon={<IconDelete />}
            size='small'
            onClick={() => removeUser(record.id)}
          >
            {t('关闭')}
          </Button>
        </Space>
      ),
    },
  ];

  const detailColumns = [
    {
      title: t('分组名'),
      dataIndex: 'name',
      key: 'name',
      render: (text) => <Text strong>{text}</Text>,
    },
    {
      title: t('系统默认倍率'),
      dataIndex: 'default_ratio',
      key: 'default_ratio',
    },
    {
      title: t('已配置'),
      dataIndex: 'configured',
      key: 'configured',
      render: (val, record) => (
        <Switch
          checked={val}
          onChange={(checked) => toggleGroupConfigured(record.name, checked)}
        />
      ),
    },
    {
      title: t('自定义倍率'),
      dataIndex: 'ratio',
      key: 'ratio',
      render: (val, record) => (
        <InputNumber
          value={record.configured ? val : undefined}
          min={0}
          step={0.1}
          disabled={!record.configured}
          onChange={(v) => updateGroupRatio(record.name, v)}
          style={{ width: 120 }}
        />
      ),
    },
  ];

  const detailTableData = detailData
    ? Object.entries(detailData.groups).map(([name, info]) => ({
        key: name,
        name,
        default_ratio: info.default_ratio,
        configured: info.configured,
        ratio: info.ratio,
      }))
    : [];

  return (
    <>
      <Card
        title={
          <Space>
            <Title heading={4} className='m-0'>
              {t('千人千面定价')}
            </Title>
          </Space>
        }
        headerExtraContent={
          <Button
            theme='solid'
            type='primary'
            icon={<IconPlus />}
            onClick={() => setAddModalVisible(true)}
          >
            {t('添加用户')}
          </Button>
        }
      >
        <Banner
          type='info'
          description={t(
            '为特定用户开启千人千面标签后，可单独设置该用户各分组的倍率。未配置的分组将使用系统默认倍率。',
          )}
          className='mb-4'
          closeIcon={null}
        />
        <Table
          columns={userColumns}
          dataSource={users}
          loading={loading}
          rowKey='id'
          pagination={false}
          empty={<Empty description={t('暂无千人千面用户')} />}
        />
      </Card>

      <SideSheet
        title={
          <Space>
            <Tag color='blue' shape='circle'>
              {t('编辑')}
            </Tag>
            <Title heading={4} className='m-0'>
              {currentUser?.username} - {t('分组定价')}
            </Title>
          </Space>
        }
        visible={detailVisible}
        onCancel={() => setDetailVisible(false)}
        width={650}
        footer={
          <div className='flex justify-end'>
            <Space>
              <Button onClick={applyAllDefaults}>{t('一键填充默认值')}</Button>
              <Button
                theme='solid'
                type='primary'
                loading={saving}
                onClick={saveDetail}
              >
                {t('保存')}
              </Button>
            </Space>
          </div>
        }
      >
        <Spin spinning={detailLoading}>
          {detailData && (
            <Table
              columns={detailColumns}
              dataSource={detailTableData}
              rowKey='name'
              pagination={false}
              rowClassName={(record) =>
                !record.configured ? 'bg-orange-50' : ''
              }
            />
          )}
        </Spin>
      </SideSheet>

      <Modal
        title={t('添加千人千面用户')}
        visible={addModalVisible}
        onOk={addUserCustomPricing}
        onCancel={() => {
          setAddModalVisible(false);
          setSelectedUserId(null);
        }}
        okText={t('确认')}
        cancelText={t('取消')}
      >
        <Select
          style={{ width: '100%' }}
          filter
          remote
          onSearch={handleSearchUser}
          loading={searchLoading}
          optionList={searchUsers}
          placeholder={t('搜索用户名')}
          onChange={(value) => setSelectedUserId(value)}
          prefix={<IconSearch />}
        />
      </Modal>
    </>
  );
};

export default CustomPricing;
