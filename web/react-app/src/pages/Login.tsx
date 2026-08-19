import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Form, Input, Button, message } from 'antd';
import { User, Lock, ArrowRight, Activity, Cpu, ShieldCheck } from 'lucide-react';
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
        setToken(token);
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
        position: 'relative',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        overflow: 'hidden',
        background: 'var(--bg-void)',
      }}
    >
      {/* Ambient aurora orbs */}
      <div
        className="sv-aurora"
        style={{
          position: 'absolute',
          width: 640,
          height: 640,
          top: '-12%',
          left: '-8%',
          borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(34,211,238,0.16), transparent 65%)',
          filter: 'blur(60px)',
          pointerEvents: 'none',
        }}
      />
      <div
        className="sv-aurora"
        style={{
          position: 'absolute',
          width: 720,
          height: 720,
          bottom: '-18%',
          right: '-10%',
          borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(192,132,252,0.16), transparent 65%)',
          filter: 'blur(70px)',
          animationDelay: '2s',
          pointerEvents: 'none',
        }}
      />
      <div
        style={{
          position: 'absolute',
          width: 420,
          height: 420,
          top: '40%',
          left: '55%',
          borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(56,189,248,0.10), transparent 60%)',
          filter: 'blur(50px)',
          pointerEvents: 'none',
        }}
      />

      {/* Floating glyphs */}
      <div
        style={{
          position: 'absolute',
          inset: 0,
          display: 'grid',
          gridTemplateColumns: 'repeat(3, 1fr)',
          placeItems: 'center',
          pointerEvents: 'none',
          opacity: 0.5,
        }}
      >
        {[<Cpu key="c" />, <ShieldCheck key="s" />, <Activity key="a" />].map((ic, i) => (
          <div key={i} className="sv-float" style={{ animationDelay: `${i * 1.4}s`, opacity: 0.25 }}>
            {ic}
          </div>
        ))}
      </div>

      {/* Center stage card */}
      <div className="sv-reveal" style={{ position: 'relative', zIndex: 2, width: '100%', maxWidth: 440, padding: '0 20px' }}>
        <div className="glass" style={{ padding: '46px 42px 38px', borderRadius: 26, boxShadow: 'var(--shadow-2), var(--glow-cyan)' }}>
          <div style={{ textAlign: 'center', marginBottom: 34 }}>
            <div style={{ display: 'inline-flex', padding: '14px 26px', borderRadius: 18, background: 'var(--grad-soft)', border: '1px solid rgba(129,140,248,0.25)' }}>
              <SuperviewLogo size={44} collapsed={false} centered />
            </div>
            <p
              className="display"
              style={{
                marginTop: 24,
                fontSize: 17,
                letterSpacing: '0.04em',
                color: 'var(--text-mid)',
              }}
            >
              {t.login.subtitle || 'Supervisor Management Platform'}
            </p>
          </div>

          <Form name="login" onFinish={onFinish} autoComplete="off" size="large">
            <Form.Item name="username" rules={[{ required: true, message: t.login.usernameRequired }]}>
              <Input
                prefix={<User size={16} strokeWidth={1.7} style={{ color: 'var(--text-low)' }} />}
                placeholder={t.login.username}
                style={{ height: 48, fontSize: 14 }}
              />
            </Form.Item>

            <Form.Item name="password" rules={[{ required: true, message: t.login.passwordRequired }]}>
              <Input.Password
                prefix={<Lock size={16} strokeWidth={1.7} style={{ color: 'var(--text-low)' }} />}
                placeholder={t.login.password}
                style={{ height: 48, fontSize: 14 }}
              />
            </Form.Item>

            <Form.Item style={{ marginTop: 26 }}>
              <Button
                type="primary"
                htmlType="submit"
                loading={loading}
                block
                style={{
                  height: 50,
                  fontSize: 15,
                  borderRadius: 12,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  gap: 8,
                }}
              >
                {t.login.loginButton}
                <ArrowRight size={17} strokeWidth={2} />
              </Button>
            </Form.Item>
          </Form>

          <div style={{ textAlign: 'center', color: 'var(--text-faint)', fontSize: 11.5, fontFamily: 'var(--font-mono)', letterSpacing: '0.03em' }}>
            {t.login.defaultCredentialsHint || 'Please contact your administrator for access.'}
          </div>
        </div>
      </div>
    </div>
  );
}