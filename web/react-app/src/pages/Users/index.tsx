import { Suspense, lazy, useEffect, useState } from 'react';
import {
  Table,
  Tag,
  Button,
  Space,
  Card,
  message,
  Form,
  Switch,
  Popconfirm,
  Avatar,
  Spin,
} from 'antd';
import {
  Plus,
  Trash2,
  RefreshCw,
  User as UserIcon,
  Settings,
} from 'lucide-react';
import type { ColumnsType } from 'antd/es/table';
import { usersApi, User, CreateUserRequest, AvailableNode, UserNodeAccess } from '../../api/users';
import { useStore } from '../../store';

const CreateUserModal = lazy(() => import('./CreateUserModal'));
const UserSettingsDrawer = lazy(() => import('./UserSettingsDrawer'));

interface EditableNodeAccess extends Pick<UserNodeAccess, 'node_id' | 'can_read' | 'can_write' | 'can_delete'> {
  node: AvailableNode;
}

function DeferredPanelFallback() {
  return (
    <div style={{ padding: 24, textAlign: 'center' }}>
      <Spin />
    </div>
  );
}

const Users: React.FC = () => {
  const { user: currentUser, t } = useStore();
  const isAdmin = currentUser?.is_admin ?? false;
  const [users, setUsers] = useState<User[]>([]);
  const [loading, setLoading] = useState(true);
  const [createModalVisible, setCreateModalVisible] = useState(false);
  const [drawerVisible, setDrawerVisible] = useState(false);
  const [selectedUser, setSelectedUser] = useState<User | null>(null);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [saving, setSaving] = useState(false);
  const [nodeAccessLoading, setNodeAccessLoading] = useState(false);
  const [restrictNodeAccess, setRestrictNodeAccess] = useState(false);
  const [nodeAccessRows, setNodeAccessRows] = useState<EditableNodeAccess[]>([]);
  const [createForm] = Form.useForm();
  const [profileForm] = Form.useForm();
  const [passwordForm] = Form.useForm();
  const [notificationForm] = Form.useForm();

  useEffect(() => {
    loadUsers();
  }, [page, pageSize]);

  const loadUsers = async () => {
    setLoading(true);
    try {
      if (isAdmin) {
        // Admin sees all users
        const response = await usersApi.getUsers(page, pageSize);
        if (response?.data) {
          setUsers(response.data.users || []);
          setTotal(response.data.total || 0);
        }
      } else {
        // Non-admin only sees themselves
        if (currentUser) {
          setUsers([{
            id: currentUser.id,
            username: currentUser.username,
            email: currentUser.email || '',
            full_name: currentUser.full_name,
            is_admin: currentUser.is_admin,
            is_active: true,
            created_at: '',
            updated_at: '',
          }]);
          setTotal(1);
        }
      }
    } catch (error) {
      console.error('Failed to load users:', error);
      message.error('Failed to load users');
    } finally {
      setLoading(false);
    }
  };

  const handleCreate = () => {
    createForm.resetFields();
    setCreateModalVisible(true);
  };

  const handleUserSettings = async (user: User) => {
    setSelectedUser(user);
    setDrawerVisible(true);
    
    // Set basic user info first
    profileForm.setFieldsValue({
      username: user.username,
      email: user.email,
      full_name: user.full_name || '',
      is_admin: user.is_admin,
      is_active: user.is_active,
      timezone: 'UTC', // Default, will be overwritten
    });
    passwordForm.resetFields();
    
    // Load user preferences from backend
    try {
      const [prefs, nodeAccessResponse] = await Promise.all([
        usersApi.getUserPreferences(user.id),
        isAdmin ? usersApi.getUserNodeAccess(user.id) : Promise.resolve(null),
      ]);

      // Update profile form with timezone
      profileForm.setFieldsValue({
        timezone: prefs.timezone || 'UTC',
      });
      // Update notification form
      notificationForm.setFieldsValue({
        email_notifications: prefs.email_notifications ?? true,
        process_alerts: prefs.process_alerts ?? true,
        system_alerts: prefs.system_alerts ?? true,
        node_status_changes: prefs.node_status_changes ?? false,
        weekly_report: prefs.weekly_report ?? false,
      });

      if (nodeAccessResponse?.data) {
        const accessByNodeId = new Map<number, UserNodeAccess>(
          nodeAccessResponse.data.node_access.map((entry) => [entry.node_id, entry])
        );
        setRestrictNodeAccess(nodeAccessResponse.data.node_access.length > 0);
        setNodeAccessRows(
          nodeAccessResponse.data.available_nodes.map((node) => {
            const existing = accessByNodeId.get(node.id);
            return {
              node_id: node.id,
              can_read: existing?.can_read ?? false,
              can_write: existing?.can_write ?? false,
              can_delete: existing?.can_delete ?? false,
              node,
            };
          })
        );
      } else {
        setRestrictNodeAccess(false);
        setNodeAccessRows([]);
      }
    } catch (error) {
      console.error('Failed to load user preferences:', error);
      // Set defaults if loading fails
      notificationForm.setFieldsValue({
        email_notifications: true,
        process_alerts: true,
        system_alerts: true,
        node_status_changes: false,
        weekly_report: false,
      });
      setRestrictNodeAccess(false);
      setNodeAccessRows([]);
    }
  };

  const handleDelete = async (userId: string) => {
    try {
      await usersApi.deleteUser(userId);
      message.success('User deleted successfully');
      loadUsers();
    } catch (error: any) {
      message.error(error.response?.data?.message || 'Failed to delete user');
    }
  };

  const handleCreateSubmit = async () => {
    try {
      const values = await createForm.validateFields();
      const createData: CreateUserRequest = {
        username: values.username,
        email: values.email,
        password: values.password,
        full_name: values.full_name,
        is_admin: values.is_admin || false,
      };
      await usersApi.createUser(createData);
      message.success('User created successfully');
      setCreateModalVisible(false);
      loadUsers();
    } catch (error: any) {
      message.error(error.response?.data?.message || 'Failed to create user');
    }
  };

  const handleProfileUpdate = async () => {
    if (!selectedUser) return;
    try {
      const values = await profileForm.validateFields();
      setSaving(true);
      
      // Update user basic info
      await usersApi.updateUser(selectedUser.id, {
        email: values.email,
        full_name: values.full_name,
        is_admin: values.is_admin,
        is_active: values.is_active,
      });
      
      // Update timezone in user preferences
      await usersApi.updateUserPreferences(selectedUser.id, {
        timezone: values.timezone,
      });
      
      message.success('User updated successfully');
      loadUsers();
      // Update selectedUser to reflect changes
      setSelectedUser({ ...selectedUser, ...values });
    } catch (error: any) {
      message.error(error.response?.data?.message || 'Failed to update user');
    } finally {
      setSaving(false);
    }
  };

  const handlePasswordChange = async () => {
    if (!selectedUser) return;
    try {
      const values = await passwordForm.validateFields();
      setSaving(true);
      
      await usersApi.resetPassword(selectedUser.id, values.new_password);
      
      message.success(t.users.passwordChanged);
      passwordForm.resetFields();
    } catch (error: any) {
      message.error(error.response?.data?.message || 'Failed to change password');
    } finally {
      setSaving(false);
    }
  };

  const handleNotificationUpdate = async () => {
    if (!selectedUser) return;
    try {
      const values = await notificationForm.validateFields();
      setSaving(true);
      
      await usersApi.updateUserPreferences(selectedUser.id, {
        email_notifications: values.email_notifications,
        process_alerts: values.process_alerts,
        system_alerts: values.system_alerts,
        node_status_changes: values.node_status_changes,
        weekly_report: values.weekly_report,
      });
      
      message.success(t.users.notificationUpdated);
    } catch (error: any) {
      message.error(error.response?.data?.message || 'Failed to update notifications');
    } finally {
      setSaving(false);
    }
  };

  const updateNodeAccessRow = (
    nodeId: number,
    field: 'can_read' | 'can_write' | 'can_delete',
    checked: boolean
  ) => {
    setNodeAccessRows((prev) =>
      prev.map((row) => {
        if (row.node_id !== nodeId) {
          return row;
        }

        if (field === 'can_read') {
          return {
            ...row,
            can_read: checked,
            can_write: checked ? row.can_write : false,
            can_delete: checked ? row.can_delete : false,
          };
        }

        return {
          ...row,
          can_read: checked ? true : row.can_read,
          [field]: checked,
        };
      })
    );
  };

  const handleNodeAccessUpdate = async () => {
    if (!selectedUser) return;

    try {
      setNodeAccessLoading(true);
      const nodeAccessPayload = restrictNodeAccess
        ? nodeAccessRows
            .filter((row) => row.can_read || row.can_write || row.can_delete)
            .map(({ node_id, can_read, can_write, can_delete }) => ({
              node_id,
              can_read,
              can_write,
              can_delete,
            }))
        : [];

      const response = await usersApi.updateUserNodeAccess(selectedUser.id, {
        node_access: nodeAccessPayload,
      });

      const accessByNodeId = new Map<number, UserNodeAccess>(
        response.data.node_access.map((entry) => [entry.node_id, entry])
      );
      setNodeAccessRows((prev) =>
        prev.map((row) => {
          const updated = accessByNodeId.get(row.node_id);
          return {
            ...row,
            can_read: updated?.can_read ?? false,
            can_write: updated?.can_write ?? false,
            can_delete: updated?.can_delete ?? false,
          };
        })
      );

      message.success(t.users.nodeAccessUpdated);
    } catch (error: any) {
      message.error(error.response?.data?.message || t.users.nodeAccessUpdateFailed);
    } finally {
      setNodeAccessLoading(false);
    }
  };

  const nodeAccessColumns: ColumnsType<EditableNodeAccess> = [
    {
      title: t.processInstance.node,
      key: 'node',
      render: (_, record) => (
        <div>
          <div style={{ fontWeight: 500 }}>{record.node.name}</div>
          <div style={{ fontSize: 12, color: '#8c8c8c' }}>
            {record.node.environment || '-'} · {record.node.host}:{record.node.port}
          </div>
        </div>
      ),
    },
    {
      title: t.users.readAccess,
      dataIndex: 'can_read',
      key: 'can_read',
      width: 96,
      render: (value: boolean, record) => (
        <Switch
          checked={value}
          disabled={!restrictNodeAccess}
          onChange={(checked) => updateNodeAccessRow(record.node_id, 'can_read', checked)}
        />
      ),
    },
    {
      title: t.users.writeAccess,
      dataIndex: 'can_write',
      key: 'can_write',
      width: 96,
      render: (value: boolean, record) => (
        <Switch
          checked={value}
          disabled={!restrictNodeAccess}
          onChange={(checked) => updateNodeAccessRow(record.node_id, 'can_write', checked)}
        />
      ),
    },
    {
      title: t.users.deleteAccess,
      dataIndex: 'can_delete',
      key: 'can_delete',
      width: 96,
      render: (value: boolean, record) => (
        <Switch
          checked={value}
          disabled={!restrictNodeAccess}
          onChange={(checked) => updateNodeAccessRow(record.node_id, 'can_delete', checked)}
        />
      ),
    },
  ];

  const columns: ColumnsType<User> = [
    {
      title: t.users.username,
      key: 'user',
      render: (_, record) => (
        <Space>
          <Avatar icon={<UserIcon size={14} strokeWidth={1.7} />} />
          <div>
            <div style={{ fontWeight: 500 }}>{record.username}</div>
            <div style={{ fontSize: '12px', color: '#999' }}>{record.email}</div>
          </div>
        </Space>
      ),
    },
    {
      title: t.common.name,
      dataIndex: 'full_name',
      key: 'full_name',
      render: (name) => name || '-',
    },
    {
      title: t.users.role,
      dataIndex: 'is_admin',
      key: 'role',
      render: (isAdmin: boolean) => (
        <Tag color={isAdmin ? 'red' : 'blue'}>
          {isAdmin ? t.users.admin : t.users.user}
        </Tag>
      ),
    },
    {
      title: t.common.status,
      dataIndex: 'is_active',
      key: 'status',
      render: (isActive: boolean) => (
        <Tag color={isActive ? 'green' : 'default'}>
          {isActive ? t.common.enabled : t.common.disabled}
        </Tag>
      ),
    },
    {
      title: t.users.lastLogin,
      dataIndex: 'last_login',
      key: 'last_login',
      render: (date) => date ? new Date(date).toLocaleString() : t.users.never,
    },
    {
      title: t.common.actions,
      key: 'actions',
      render: (_, record) => (
        <Space>
          <Button
            type="link"
            icon={<Settings size={14} strokeWidth={1.7} />}
            onClick={() => handleUserSettings(record)}
          >
            {t.nav.settings}
          </Button>
          {isAdmin && (
            <Popconfirm
              title={t.users.deleteUser}
              description={t.users.confirmDelete}
              onConfirm={() => handleDelete(record.id)}
              okText={t.common.yes}
              cancelText={t.common.no}
              disabled={record.is_admin}
            >
              <Button type="link" danger icon={<Trash2 size={14} strokeWidth={1.7} />} disabled={record.is_admin}>
                {t.common.delete}
              </Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ];

  return (
    <div style={{ padding: '24px' }}>
      <Card
        title={t.users.title}
        extra={
          <Space>
            <Button icon={<RefreshCw size={14} strokeWidth={1.7} />} onClick={loadUsers} loading={loading}>
              {t.common.refresh}
            </Button>
            {isAdmin && (
              <Button type="primary" icon={<Plus size={14} strokeWidth={1.7} />} onClick={handleCreate}>
                {t.users.addUser}
              </Button>
            )}
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={users}
          rowKey="id"
          loading={loading}
          pagination={{
            current: page,
            pageSize: pageSize,
            total: total,
            showSizeChanger: true,
            showTotal: (total) => `${t.common.total} ${total}`,
            onChange: (p, ps) => { setPage(p); setPageSize(ps); },
          }}
        />
      </Card>

      {createModalVisible && (
        <Suspense fallback={<DeferredPanelFallback />}>
          <CreateUserModal
            open={createModalVisible}
            form={createForm}
            t={t}
            onSubmit={handleCreateSubmit}
            onCancel={() => setCreateModalVisible(false)}
          />
        </Suspense>
      )}

      {drawerVisible && selectedUser && (
        <Suspense fallback={<DeferredPanelFallback />}>
          <UserSettingsDrawer
            t={t}
            selectedUser={selectedUser}
            open={drawerVisible}
            onClose={() => setDrawerVisible(false)}
            isAdmin={isAdmin}
            saving={saving}
            nodeAccessLoading={nodeAccessLoading}
            profileForm={profileForm}
            passwordForm={passwordForm}
            notificationForm={notificationForm}
            restrictNodeAccess={restrictNodeAccess}
            setRestrictNodeAccess={setRestrictNodeAccess}
            nodeAccessRows={nodeAccessRows}
            nodeAccessColumns={nodeAccessColumns}
            onProfileUpdate={handleProfileUpdate}
            onPasswordChange={handlePasswordChange}
            onNotificationUpdate={handleNotificationUpdate}
            onNodeAccessUpdate={handleNodeAccessUpdate}
          />
        </Suspense>
      )}
    </div>
  );
};

export default Users;
