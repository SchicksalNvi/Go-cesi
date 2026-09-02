import { useState } from 'react';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { Layout, Menu, Avatar, Dropdown, Button } from 'antd';
import {
  LayoutDashboard,
  Boxes,
  Server,
  Layers,
  Radar,
  FileText,
  Users,
  Settings,
  LogOut,
  PanelLeftClose,
  PanelLeftOpen,
  Languages,
  UserRound,
  Activity,
} from 'lucide-react';
import type { MenuProps } from 'antd';
import { useStore } from '@/store';
import { SuperviewLogo } from '@/components/SuperviewLogo';
import type { Language } from '@/i18n';
import { hasPermission, PERMISSIONS } from '@/utils/permissions';

const { Header, Sider, Content } = Layout;

// Lucide icon renderer adapter (stroke-based, 1.6px, consistent 18px)
const icon = (Icon: typeof LayoutDashboard, size = 17) => (
  <Icon size={size} strokeWidth={1.7} />
);

export default function MainLayout() {
  const navigate = useNavigate();
  const location = useLocation();
  const { user, logout, t, language, setLanguage } = useStore();
  const [collapsed, setCollapsed] = useState(false);
  const canReadLogs = hasPermission(user, PERMISSIONS.logRead);
  const canReadUsers = hasPermission(user, PERMISSIONS.userRead);
  const canManageDiscovery = hasPermission(user, PERMISSIONS.systemManage);

  const handleLanguageSwitch = () => {
    setLanguage(language === 'en' ? 'zh' : 'en');
  };

  const baseMenuItems: MenuProps['items'] = [
    {
      key: '/dashboard',
      icon: icon(LayoutDashboard),
      label: t.nav.dashboard,
      onClick: () => navigate('/dashboard'),
    },
    {
      key: '/environments',
      icon: icon(Boxes),
      label: t.nav.environments,
      onClick: () => navigate('/environments'),
    },
    {
      key: '/nodes',
      icon: icon(Server),
      label: t.nav.nodes,
      onClick: () => navigate('/nodes'),
    },
    {
      key: '/processes',
      icon: icon(Layers),
      label: t.nav.processes,
      onClick: () => navigate('/processes'),
    },
    ...(canManageDiscovery
      ? [{
          key: '/discovery',
          icon: icon(Radar),
          label: t.nav.discovery,
          onClick: () => navigate('/discovery'),
        }]
      : []),
    ...(canReadLogs
      ? [{
          key: '/logs',
          icon: icon(FileText),
          label: t.nav.logs,
          onClick: () => navigate('/logs'),
        }]
      : []),
    ...(canReadUsers
      ? [{
          key: '/users',
          icon: icon(Users),
          label: t.nav.users,
          onClick: () => navigate('/users'),
        }]
      : []),
  ];

  const adminMenuItems: MenuProps['items'] = user?.is_admin ? [
    {
      key: '/settings',
      icon: icon(Settings),
      label: t.nav.settings,
      onClick: () => navigate('/settings'),
    },
  ] : [];

  const menuItems = [...baseMenuItems, ...adminMenuItems];

  const userMenuItems: MenuProps['items'] = [
    {
      key: 'profile',
      icon: icon(UserRound),
      label: t.nav.profile,
      onClick: () => navigate('/profile'),
    },
    ...(user?.is_admin
      ? [
          {
            key: 'settings',
            icon: icon(Settings),
            label: t.nav.settings,
            onClick: () => navigate('/settings'),
          },
        ]
      : []),
    {
      type: 'divider',
    },
    {
      key: 'logout',
      icon: icon(LogOut),
      label: t.nav.logout,
      onClick: () => {
        logout();
        navigate('/login');
      },
    },
  ];

  const menuKeys = menuItems
    .map((item) => ('key' in (item || {}) ? item?.key : undefined))
    .filter((key): key is string => typeof key === 'string' && key.startsWith('/'));

  const selectedKey = menuKeys.find(
    (key) => location.pathname === key || location.pathname.startsWith(`${key}/`)
  );

  const siderWidth = collapsed ? 76 : 232;

  return (
    <Layout style={{ minHeight: '100vh', background: 'transparent' }}>
      {/* Immersive fixed glass sider */}
      <Sider
        trigger={null}
        collapsible
        collapsed={collapsed}
        width={232}
        collapsedWidth={76}
        style={{
          overflow: 'hidden',
          height: 'calc(100vh - 24px)',
          position: 'fixed',
          left: 12,
          top: 12,
          bottom: 12,
          zIndex: 100,
          background: 'rgba(10,12,20,0.55)',
          border: '1px solid var(--hairline)',
          borderRadius: 20,
          backdropFilter: 'blur(22px) saturate(150%)',
          WebkitBackdropFilter: 'blur(22px) saturate(150%)',
          boxShadow: 'var(--shadow-2)',
          transition: 'width 0.32s var(--ease-out-expo)',
        }}
      >
        <div
          style={{
            height: 68,
            display: 'flex',
            alignItems: 'center',
            justifyContent: collapsed ? 'center' : 'flex-start',
            cursor: 'pointer',
            padding: collapsed ? '0' : '0 20px',
            borderBottom: '1px solid var(--hairline)',
          }}
          onClick={() => navigate('/dashboard')}
        >
          <SuperviewLogo size={34} collapsed={collapsed} textColor="#fff" />
        </div>

        {/* Collapse toggle */}
        <div style={{ display: 'flex', justifyContent: 'center', padding: '10px 0 4px' }}>
          <Button
            type="text"
            onClick={() => setCollapsed(!collapsed)}
            style={{
              width: '90%',
              height: 34,
              borderRadius: 10,
              color: 'var(--text-low)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: collapsed ? 'center' : 'flex-start',
              flexWrap: 'nowrap',
              gap: 8,
              padding: collapsed ? 0 : '0 12px',
              lineHeight: '34px',
            }}
          >
            {collapsed
              ? <PanelLeftOpen size={17} strokeWidth={1.7} />
              : <PanelLeftClose size={17} strokeWidth={1.7} />}
          </Button>
        </div>

        <Menu
          theme="dark"
          mode="inline"
          inlineCollapsed={collapsed}
          selectedKeys={selectedKey ? [selectedKey] : []}
          items={menuItems}
          style={{
            background: 'transparent',
            border: 'none',
            padding: '6px 10px',
            fontSize: 13.5,
          }}
        />

        {/* Sider footer — system pulse */}
        <div
          style={{
            position: 'absolute',
            bottom: 16,
            left: 0,
            right: 0,
            padding: '0 18px',
          }}
        >
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 10,
              padding: '10px 12px',
              borderRadius: 12,
              background: 'var(--grad-soft)',
              border: '1px solid rgba(129,140,248,0.22)',
              ...(collapsed ? { justifyContent: 'center', padding: '10px 0' } : {}),
            }}
          >
            <Activity size={15} strokeWidth={1.7} style={{ color: 'var(--ok)' }} />
            {!collapsed && (
              <span style={{ fontSize: 11.5, color: 'var(--text-low)', fontFamily: 'var(--font-mono)' }}>
                OBSIDIAN CORE
              </span>
            )}
          </div>
        </div>
      </Sider>

      {/* Main column */}
      <Layout
        style={{
          marginLeft: siderWidth + 24,
          marginRight: 12,
          minHeight: '100vh',
          background: 'transparent',
          transition: 'margin-left 0.32s var(--ease-out-expo)',
        }}
      >
        {/* Floating glass header */}
        <Header
          style={{
            position: 'sticky',
            top: 12,
            zIndex: 90,
            height: 60,
            padding: '0 20px',
            marginTop: 12,
            background: 'rgba(10,12,20,0.55)',
            border: '1px solid var(--hairline)',
            borderRadius: 16,
            backdropFilter: 'blur(20px) saturate(150%)',
            WebkitBackdropFilter: 'blur(20px) saturate(150%)',
            boxShadow: 'var(--shadow-1)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          {/* Breadcrumb-ish current section marker */}
          <div className="hero-kicker" style={{ fontSize: 11 }}>
            {selectedKey?.replace('/', '')?.toUpperCase() || 'SUPERVIEW'}
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <Button
              type="text"
              onClick={handleLanguageSwitch}
              title={t.language.switchLanguage}
              style={{
                height: 38,
                borderRadius: 10,
                color: 'var(--text-mid)',
                display: 'flex',
                alignItems: 'center',
                gap: 6,
                fontFamily: 'var(--font-mono)',
                fontSize: 12.5,
              }}
            >
              <Languages size={16} strokeWidth={1.7} />
              {t.language[language].toUpperCase()}
            </Button>

            <Dropdown menu={{ items: userMenuItems }} placement="bottomRight" trigger={['click']}>
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 10,
                  cursor: 'pointer',
                  padding: '5px 12px 5px 6px',
                  borderRadius: 12,
                  background: 'var(--glass)',
                  border: '1px solid var(--hairline)',
                  transition: 'all 0.2s var(--ease-out-expo)',
                }}
                className="header-user-chip"
              >
                <Avatar
                  style={{
                    background: 'var(--grad-primary)',
                    fontWeight: 600,
                    fontSize: 13,
                    color: '#05060a',
                  }}
                  size={30}
                >
                  {(user?.full_name || user?.username || 'U').slice(0, 1).toUpperCase()}
                </Avatar>
                <span style={{ color: 'var(--text-hi)', fontSize: 13, fontWeight: 500 }}>
                  {user?.full_name || user?.username || 'User'}
                </span>
              </div>
            </Dropdown>
          </div>
        </Header>

        <Content
          style={{
            padding: '20px 4px 40px',
            minHeight: 280,
            background: 'transparent',
          }}
        >
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}