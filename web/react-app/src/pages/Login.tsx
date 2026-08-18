import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Form, Input, Button, Card, message, Space } from 'antd';
import { UserOutlined, LockOutlined } from '@ant-design/icons';
import { authApi } from '@/api/auth';
import { useStore } from '@/store';
import { SuperviewLogo } from '@/components/SuperviewLogo';

export default function Login() {
  const navigate = useNavigate();
  const { setUser, setToken, t } = useStore();
  const [loading, setLoading] = useState(false);

  const onFinish = async (values: { username: string; password: string }) => {
    setLoading(true);
    try {
      const response = await authApi.login(values);
      
      if (response.status === 'success' && response.data) {
        const { token, user } = response.data;
        
        // 设置 token
        setToken(token);
        
        // 设置用户信息
        setUser(user);
        
        message.success(t.login.loginSuccess);
        navigate('/dashboard');
      } else {
        message.error(response.message || t.login.loginFailed);
      }
    } catch (error: any) {
      message.error(error.response?.data?.message || t.login.loginFailed);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
      }}
    >
      <Card
        style={{
          width: 400,
          boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
        }}
      >
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <div style={{ textAlign: 'center' }}>
            <SuperviewLogo size={64} collapsed={false} centered />
            <p style={{ color: '#666', marginTop: 16 }}>
              Supervisor Management Platform
            </p>
          </div>

          <Form
            name="login"
            onFinish={onFinish}
            autoComplete="off"
            size="large"
          >
            <Form.Item
              name="username"
              rules={[{ required: true, message: t.login.usernameRequired }]}
            >
              <Input
                prefix={<UserOutlined />}
                placeholder={t.login.username}
              />
            </Form.Item>

            <Form.Item
              name="password"
              rules={[{ required: true, message: t.login.passwordRequired }]}
            >
              <Input.Password
                prefix={<LockOutlined />}
                placeholder={t.login.password}
              />
            </Form.Item>

            <Form.Item>
              <Button
                type="primary"
                htmlType="submit"
                loading={loading}
                block
              >
                {t.login.loginButton}
              </Button>
            </Form.Item>
          </Form>

          <div style={{ textAlign: 'center', color: '#999', fontSize: 12 }}>
            {t.login.defaultCredentialsHint || 'Please contact your administrator for access.'}
          </div>
        </Space>
      </Card>
    </div>
  );
}
