import { useState } from 'react';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import { Layout, Menu, Avatar, Dropdown, Space, Button } from 'antd';
import {
  DashboardOutlined,
  ClusterOutlined,
  AppstoreOutlined,
  UserOutlined,
  FileTextOutlined,
  SettingOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  RadarChartOutlined,
  GlobalOutlined,
  IdcardOutlined,
} from '@ant-design/icons';
import type { MenuProps } from 'antd';
import { useStore } from '@/store';
import { SuperviewLogo } from '@/components/SuperviewLogo';
import type { Language } from '@/i18n';
import { hasPermission, PERMISSIONS } from '@/utils/permissions';

const { Header, Sider, Content } = Layout;

export default function MainLayout() {
  const navigate = useNavigate();
  const location = useLocation();
  const { user, logout, t, language, setLanguage } = useStore();
  const [collapsed, setCollapsed] = useState(false);
  const canReadLogs = hasPermission(user, PERMISSIONS.logRead);
  const canReadUsers = hasPermission(user, PERMISSIONS.userRead);
  const canManageDiscovery = hasPermission(user, PERMISSIONS.systemManage);

  // Language switch handler
  const handleLanguageSwitch = () => {
    setLanguage(language === 'en' ? 'zh' : 'en');
  };

  const baseMenuItems: MenuProps['items'] = [
    {
      key: '/dashboard',
      icon: <DashboardOutlined />,
      label: t.nav.dashboard,
      onClick: () => navigate('/dashboard'),
    },
    {
      key: '/environments',
      icon: <AppstoreOutlined />,
      label: t.nav.environments,
      onClick: () => navigate('/environments'),
    },
    {
      key: '/nodes',
      icon: <ClusterOutlined />,
      label: t.nav.nodes,
      onClick: () => navigate('/nodes'),
    },
    {
      key: '/processes',
      icon: <AppstoreOutlined />,
      label: t.nav.processes,
      onClick: () => navigate('/processes'),
    },
    ...(canManageDiscovery
      ? [{
          key: '/discovery',
          icon: <RadarChartOutlined />,
          label: t.nav.discovery,
          onClick: () => navigate('/discovery'),
        }]
      : []),
    ...(canReadLogs
      ? [{
          key: '/logs',
          icon: <FileTextOutlined />,
          label: t.nav.logs,
          onClick: () => navigate('/logs'),
        }]
      : []),
    ...(canReadUsers
      ? [{
          key: '/users',
          icon: <UserOutlined />,
          label: t.nav.users,
          onClick: () => navigate('/users'),
        }]
      : []),
  ];

  // Admin-only menu items
  const adminMenuItems: MenuProps['items'] = user?.is_admin ? [
    {
      key: '/settings',
      icon: <SettingOutlined />,
      label: t.nav.settings,
      onClick: () => navigate('/settings'),
    },
  ] : [];

  const menuItems = [...baseMenuItems, ...adminMenuItems];

  // User dropdown menu
  const userMenuItems: MenuProps['items'] = [
    {
      key: 'profile',
      icon: <IdcardOutlined />,
      label: t.nav.profile,
      onClick: () => navigate('/profile'),
    },
    ...(user?.is_admin
      ? [
          {
            key: 'settings',
            icon: <SettingOutlined />,
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
      icon: <LogoutOutlined />,
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

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        trigger={null}
        collapsible
        collapsed={collapsed}
        style={{
          overflow: 'auto',
          height: '100vh',
          position: 'fixed',
          left: 0,
          top: 0,
          bottom: 0,
        }}
      >
        <div
          style={{
            height: 64,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            cursor: 'pointer',
            padding: '0 20px',
          }}
          onClick={() => navigate('/dashboard')}
        >
          <SuperviewLogo size={36} collapsed={collapsed} textColor="#fff" />
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={selectedKey ? [selectedKey] : []}
          items={menuItems}
        />
      </Sider>
      
      <Layout style={{ marginLeft: collapsed ? 80 : 200, transition: 'all 0.2s' }}>
        <Header
          style={{
            padding: '0 24px',
            background: '#fff',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            boxShadow: '0 1px 4px rgba(0,21,41,.08)',
          }}
        >
          <div>
            {collapsed ? (
              <MenuUnfoldOutlined
                style={{ fontSize: 18, cursor: 'pointer' }}
                onClick={() => setCollapsed(false)}
              />
            ) : (
              <MenuFoldOutlined
                style={{ fontSize: 18, cursor: 'pointer' }}
                onClick={() => setCollapsed(true)}
              />
            )}
          </div>
          
          <Space size="large">
            <Button
              type="text"
              icon={<GlobalOutlined />}
              onClick={handleLanguageSwitch}
              title={t.language.switchLanguage}
            >
              {t.language[language]}
            </Button>
            <Dropdown menu={{ items: userMenuItems }} placement="bottomRight">
              <Space style={{ cursor: 'pointer' }}>
                <Avatar icon={<UserOutlined />} />
                <span>{user?.full_name || user?.username || 'User'}</span>
              </Space>
            </Dropdown>
          </Space>
        </Header>
        
        <Content
          style={{
            margin: '24px 16px',
            padding: 24,
            minHeight: 280,
            background: '#f0f2f5',
          }}
        >
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}
