import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Card, Col, Form, Input, Row, Space, Typography, message } from 'antd';
import { LockOutlined, MailOutlined, SaveOutlined, UserOutlined } from '@ant-design/icons';
import { authApi } from '@/api/auth';
import { useStore } from '@/store';

const { Text } = Typography;

export default function Profile() {
  const { user, setUser, t } = useStore();
  const [profileForm] = Form.useForm();
  const [passwordForm] = Form.useForm();
  const [loadingProfile, setLoadingProfile] = useState(false);
  const [savingProfile, setSavingProfile] = useState(false);
  const [savingPassword, setSavingPassword] = useState(false);
  // Guards against the load->setUser->re-render->load loop: only fetch the
  // profile once per user (and again if a different user logs in).
  const loadedUserIdRef = useRef<string | null>(null);

  useEffect(() => {
    if (user) {
      profileForm.setFieldsValue({
        username: user.username,
        email: user.email || '',
        full_name: user.full_name || '',
      });
      if (loadedUserIdRef.current !== user.id) {
        loadedUserIdRef.current = user.id;
        loadProfile();
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [user, profileForm]);

  const loadProfile = async () => {
    setLoadingProfile(true);
    try {
      const response = await authApi.getProfile();
      const profileUser = response.data?.user;
      if (profileUser) {
        setUser(profileUser);
        profileForm.setFieldsValue({
          username: profileUser.username,
          email: profileUser.email || '',
          full_name: profileUser.full_name || '',
        });
      }
    } catch (error) {
      console.error('Failed to load profile:', error);
      message.error(t.profile.loadProfileFailed);
    } finally {
      setLoadingProfile(false);
    }
  };

  const handleProfileSubmit = async () => {
    try {
      const values = await profileForm.validateFields();
      setSavingProfile(true);
      await authApi.updateProfile({
        email: values.email,
        full_name: values.full_name,
      });

      const updatedUser = {
        ...(user || {}),
        email: values.email,
        full_name: values.full_name,
      };
      setUser(updatedUser as any);
      message.success(t.profile.profileUpdated);
    } catch (error: any) {
      if (error?.errorFields) {
        return;
      }
      console.error('Failed to update profile:', error);
      message.error(error.response?.data?.message || t.profile.updateProfileFailed);
    } finally {
      setSavingProfile(false);
    }
  };

  const handlePasswordSubmit = async () => {
    try {
      const values = await passwordForm.validateFields();
      setSavingPassword(true);
      await authApi.changeOwnPassword({
        old_password: values.old_password,
        new_password: values.new_password,
      });
      passwordForm.resetFields();
      message.success(t.profile.passwordChanged);
    } catch (error: any) {
      if (error?.errorFields) {
        return;
      }
      console.error('Failed to change password:', error);
      message.error(error.response?.data?.message || t.profile.changePasswordFailed);
    } finally {
      setSavingPassword(false);
    }
  };

  return (
    <div>
      <h1 style={{ marginBottom: 16 }}>{t.profile.title}</h1>

      <Row gutter={[24, 24]}>
        <Col xs={24} lg={14}>
          <Card
            title={t.profile.personalInfo}
            extra={
              <Button icon={<SaveOutlined />} onClick={loadProfile} loading={loadingProfile}>
                {t.common.refresh}
              </Button>
            }
          >
            <Form form={profileForm} layout="vertical">
              <Form.Item label={t.users.username} name="username">
                <Input prefix={<UserOutlined />} disabled />
              </Form.Item>

              <Form.Item
                label={t.users.email}
                name="email"
                rules={[
                  { required: true, message: t.users.pleaseEnterEmail },
                  { type: 'email', message: t.users.pleaseEnterValidEmail },
                ]}
              >
                <Input prefix={<MailOutlined />} />
              </Form.Item>

              <Form.Item
                label={t.users.fullName}
                name="full_name"
              >
                <Input prefix={<UserOutlined />} placeholder={t.users.enterFullName} />
              </Form.Item>

              <Form.Item>
                <Button
                  type="primary"
                  icon={<SaveOutlined />}
                  onClick={handleProfileSubmit}
                  loading={savingProfile}
                >
                  {t.profile.updateProfile}
                </Button>
              </Form.Item>
            </Form>
          </Card>
        </Col>

        <Col xs={24} lg={10}>
          <Card title={t.profile.changePassword}>
            <Alert
              type="info"
              showIcon
              style={{ marginBottom: 16 }}
              message={t.profile.passwordSecurityHint}
            />

            <Form form={passwordForm} layout="vertical">
              <Form.Item
                label={t.profile.currentPassword}
                name="old_password"
                rules={[{ required: true, message: t.profile.enterCurrentPassword }]}
              >
                <Input.Password prefix={<LockOutlined />} />
              </Form.Item>

              <Form.Item
                label={t.profile.newPassword}
                name="new_password"
                rules={[
                  { required: true, message: t.users.pleaseEnterNewPassword },
                  { min: 6, message: t.users.passwordMinLength },
                ]}
              >
                <Input.Password prefix={<LockOutlined />} />
              </Form.Item>

              <Form.Item
                label={t.profile.confirmNewPassword}
                name="confirm_new_password"
                dependencies={['new_password']}
                rules={[
                  { required: true, message: t.users.pleaseConfirmPassword },
                  ({ getFieldValue }) => ({
                    validator(_, value) {
                      if (!value || getFieldValue('new_password') === value) {
                        return Promise.resolve();
                      }
                      return Promise.reject(new Error(t.users.passwordMismatch));
                    },
                  }),
                ]}
              >
                <Input.Password prefix={<LockOutlined />} />
              </Form.Item>

              <Form.Item>
                <Space direction="vertical" style={{ width: '100%' }}>
                  <Button
                    type="primary"
                    icon={<LockOutlined />}
                    onClick={handlePasswordSubmit}
                    loading={savingPassword}
                  >
                    {t.profile.changePassword}
                  </Button>
                  <Text type="secondary">{t.profile.passwordUpdateNotice}</Text>
                </Space>
              </Form.Item>
            </Form>
          </Card>
        </Col>
      </Row>
    </div>
  );
}
