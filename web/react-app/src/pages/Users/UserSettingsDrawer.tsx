import {
  Alert,
  Button,
  Divider,
  Drawer,
  Form,
  Input,
  Select,
  Space,
  Switch,
  Table,
  Tabs,
} from 'antd';
import {
  Bell,
  Boxes,
  Lock,
  Mail,
  Save,
  User,
} from 'lucide-react';

interface UserSettingsDrawerProps {
  t: any;
  selectedUser: any;
  open: boolean;
  onClose: () => void;
  isAdmin: boolean;
  saving: boolean;
  nodeAccessLoading: boolean;
  profileForm: any;
  passwordForm: any;
  notificationForm: any;
  restrictNodeAccess: boolean;
  setRestrictNodeAccess: (checked: boolean) => void;
  nodeAccessRows: any[];
  nodeAccessColumns: any[];
  onProfileUpdate: () => void;
  onPasswordChange: () => void;
  onNotificationUpdate: () => void;
  onNodeAccessUpdate: () => void;
}

const UserSettingsDrawer: React.FC<UserSettingsDrawerProps> = ({
  t,
  selectedUser,
  open,
  onClose,
  isAdmin,
  saving,
  nodeAccessLoading,
  profileForm,
  passwordForm,
  notificationForm,
  restrictNodeAccess,
  setRestrictNodeAccess,
  nodeAccessRows,
  nodeAccessColumns,
  onProfileUpdate,
  onPasswordChange,
  onNotificationUpdate,
  onNodeAccessUpdate,
}) => {
  return (
    <Drawer
      title={`${t.users.userSettings}: ${selectedUser?.username || ''}`}
      placement="right"
      width={520}
      onClose={onClose}
      open={open}
    >
      <Tabs
        items={[
          {
            key: 'profile',
            label: <span><User size={14} strokeWidth={1.7} /> {t.users.profile}</span>,
            children: (
              <Form form={profileForm} layout="vertical">
                <Form.Item name="username" label={t.users.username}>
                  <Input prefix={<User size={14} strokeWidth={1.7} />} disabled />
                </Form.Item>
                <Form.Item
                  name="email"
                  label={t.users.email}
                  rules={[
                    { required: true, message: t.users.pleaseEnterEmail },
                    { type: 'email', message: t.users.pleaseEnterValidEmail },
                  ]}
                >
                  <Input prefix={<Mail size={14} strokeWidth={1.7} />} />
                </Form.Item>
                <Form.Item name="full_name" label={t.users.fullName}>
                  <Input placeholder={t.users.enterFullName} />
                </Form.Item>
                <Form.Item name="timezone" label={t.settings.timezone}>
                  <Select
                    options={[
                      { label: 'UTC', value: 'UTC' },
                      { label: 'America/New_York', value: 'America/New_York' },
                      { label: 'America/Los_Angeles', value: 'America/Los_Angeles' },
                      { label: 'Europe/London', value: 'Europe/London' },
                      { label: 'Asia/Shanghai', value: 'Asia/Shanghai' },
                      { label: 'Asia/Tokyo', value: 'Asia/Tokyo' },
                    ]}
                  />
                </Form.Item>
                {isAdmin && (
                  <>
                    <Divider />
                    <Form.Item name="is_admin" label={t.users.adminRole} valuePropName="checked">
                      <Switch checkedChildren={t.users.admin} unCheckedChildren={t.users.user} />
                    </Form.Item>
                    <Form.Item name="is_active" label={t.users.accountStatus} valuePropName="checked">
                      <Switch checkedChildren={t.users.active} unCheckedChildren={t.users.inactive} />
                    </Form.Item>
                  </>
                )}
                <Form.Item>
                  <Button type="primary" icon={<Save size={14} strokeWidth={1.7} />} onClick={onProfileUpdate} loading={saving}>
                    {t.users.saveProfile}
                  </Button>
                </Form.Item>
              </Form>
            ),
          },
          {
            key: 'security',
            label: <span><Lock size={14} strokeWidth={1.7} /> {t.users.security}</span>,
            children: (
              <Form form={passwordForm} layout="vertical">
                <Form.Item
                  name="new_password"
                  label={t.users.newPassword}
                  rules={[
                    { required: true, message: t.users.pleaseEnterNewPassword },
                    { min: 6, message: t.users.passwordMinLength },
                  ]}
                >
                  <Input.Password prefix={<Lock size={14} strokeWidth={1.7} />} placeholder={t.users.enterNewPassword} />
                </Form.Item>
                <Form.Item
                  name="confirm_password"
                  label={t.users.confirmPassword}
                  dependencies={['new_password']}
                  rules={[
                    { required: true, message: t.users.pleaseConfirmPassword },
                    ({ getFieldValue }: any) => ({
                      validator(_: unknown, value: string) {
                        if (!value || getFieldValue('new_password') === value) {
                          return Promise.resolve();
                        }
                        return Promise.reject(new Error(t.users.passwordMismatch));
                      },
                    }),
                  ]}
                >
                  <Input.Password prefix={<Lock size={14} strokeWidth={1.7} />} placeholder={t.users.confirmNewPassword} />
                </Form.Item>
                <Form.Item>
                  <Button type="primary" icon={<Save size={14} strokeWidth={1.7} />} onClick={onPasswordChange} loading={saving}>
                    {t.users.resetPassword}
                  </Button>
                </Form.Item>
              </Form>
            ),
          },
          {
            key: 'notifications',
            label: <span><Bell size={14} strokeWidth={1.7} /> {t.users.notifications}</span>,
            children: (
              <Form form={notificationForm} layout="vertical">
                <Form.Item name="email_notifications" label={t.users.emailNotifications} valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Divider />
                <Form.Item name="process_alerts" label={t.users.processAlerts} valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.Item name="system_alerts" label={t.users.systemAlerts} valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.Item name="node_status_changes" label={t.users.nodeStatusChanges} valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Divider />
                <Form.Item name="weekly_report" label={t.users.weeklyReport} valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.Item>
                  <Button type="primary" icon={<Save size={14} strokeWidth={1.7} />} onClick={onNotificationUpdate} loading={saving}>
                    {t.users.savePreferences}
                  </Button>
                </Form.Item>
              </Form>
            ),
          },
          ...(isAdmin ? [{
            key: 'node-access',
            label: <span><Boxes size={14} strokeWidth={1.7} /> {t.users.nodeAccess}</span>,
            children: (
              <Space direction="vertical" size="middle" style={{ width: '100%' }}>
                <Alert
                  type="info"
                  showIcon
                  message={restrictNodeAccess ? t.users.nodeAccessRestricted : t.users.nodeAccessInherited}
                  description={restrictNodeAccess ? t.users.nodeAccessRestrictedHelp : t.users.nodeAccessInheritedHelp}
                />
                <div
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    gap: 16,
                    flexWrap: 'wrap',
                  }}
                >
                  <div>
                    <div style={{ fontWeight: 500 }}>{t.users.restrictNodeAccess}</div>
                    <div style={{ fontSize: 12, color: '#8c8c8c' }}>{t.users.restrictNodeAccessHelp}</div>
                  </div>
                  <Switch checked={restrictNodeAccess} onChange={setRestrictNodeAccess} />
                </div>
                <Table
                  size="small"
                  rowKey="node_id"
                  columns={nodeAccessColumns}
                  dataSource={nodeAccessRows}
                  pagination={false}
                  locale={{ emptyText: t.users.noNodesAvailable }}
                />
                <Button
                  type="primary"
                  icon={<Save size={14} strokeWidth={1.7} />}
                  onClick={onNodeAccessUpdate}
                  loading={nodeAccessLoading}
                >
                  {t.users.saveNodeAccess}
                </Button>
              </Space>
            ),
          }] : []),
        ]}
      />
    </Drawer>
  );
};

export default UserSettingsDrawer;
