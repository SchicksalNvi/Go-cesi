import { Suspense, lazy, useEffect } from 'react';
import { Routes, Route, Navigate } from 'react-router-dom';
import { Spin } from 'antd';
import { useStore } from '@/store';
import { authApi } from '@/api/auth';
import { settingsApi } from '@/api/settings';
import MainLayout from '@/layouts/MainLayout';
import { hasAnyPermission, PERMISSIONS } from '@/utils/permissions';

const Login = lazy(() => import('@/pages/Login'));
const Dashboard = lazy(() => import('@/pages/Dashboard'));
const NodeList = lazy(() => import('@/pages/Nodes'));
const NodeDetail = lazy(() => import('@/pages/Nodes/NodeDetail'));
const ProcessesPage = lazy(() => import('@/pages/Processes'));
const EnvironmentList = lazy(() => import('@/pages/Environments'));
const EnvironmentDetail = lazy(() => import('@/pages/Environments/EnvironmentDetail'));
const UserList = lazy(() => import('@/pages/Users'));
const LogList = lazy(() => import('@/pages/Logs'));
const Settings = lazy(() => import('@/pages/Settings'));
const DiscoveryPage = lazy(() => import('@/pages/Discovery'));
const ProfilePage = lazy(() => import('@/pages/Profile'));

// Protected Route Component
function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = useStore();
  
  if (!isAuthenticated) {
    return <Navigate to="/login" replace />;
  }
  
  return <>{children}</>;
}

function RouteFallback() {
  return (
    <div style={{ minHeight: '50vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
      <Spin size="large" />
    </div>
  );
}

function App() {
  const { isAuthenticated, user, setUser, logout, setWebsocketEnabled } = useStore();

  // Listen for logout events from API interceptor
  useEffect(() => {
    const handleLogout = () => {
      logout();
    };

    window.addEventListener('auth:logout', handleLogout);
    return () => window.removeEventListener('auth:logout', handleLogout);
  }, [logout]);

  // Load system settings (including WebSocket enabled) on auth
  useEffect(() => {
    const loadSystemSettings = async () => {
      try {
        const response = await settingsApi.getSystemSettings();
        const settings = response.settings || {};
        const wsEnabled = settings.enable_websocket !== 'false';
        setWebsocketEnabled(wsEnabled);
      } catch (error) {
        // Use default (enabled) if can't load settings
        console.error('Failed to load system settings:', error);
      }
    };

    if (!isAuthenticated) {
      return;
    }

    if (!hasAnyPermission(user, [PERMISSIONS.systemConfig, PERMISSIONS.systemManage])) {
      setWebsocketEnabled(true);
      return;
    }

    loadSystemSettings();
  }, [isAuthenticated, user, setWebsocketEnabled]);

  // Validate token on mount only if we have both token and user
  useEffect(() => {
    const token = localStorage.getItem('token');
    const user = localStorage.getItem('user');
    
    if (token && user) {
      // We have stored credentials, verify they're still valid
      loadUserInfo();
    } else if (token || user) {
      // Partial credentials, clean up
      logout();
    }
  }, []);

  const loadUserInfo = async () => {
    try {
      const response = await authApi.getCurrentUser();
      // 后端返回 { status, data: { user: {...} } }
      if (response.status === 'success' && (response as any).data?.user) {
        setUser((response as any).data.user);
      } else {
        // Invalid response, logout
        logout();
      }
    } catch (error) {
      console.error('Failed to load user info:', error);
      // Token is invalid, logout
      logout();
    }
  };

  return (
    <Suspense fallback={<RouteFallback />}>
      <Routes>
        <Route path="/login" element={<Login />} />
        
        <Route
          path="/"
          element={
            <ProtectedRoute>
              <MainLayout />
            </ProtectedRoute>
          }
        >
          <Route index element={<Navigate to="/dashboard" replace />} />
          <Route path="dashboard" element={<Dashboard />} />
          <Route path="nodes" element={<NodeList />} />
          <Route path="nodes/:nodeName" element={<NodeDetail />} />
          <Route path="processes" element={<ProcessesPage />} />
          <Route path="environments" element={<EnvironmentList />} />
          <Route path="environments/:environmentName" element={<EnvironmentDetail />} />
          <Route path="users" element={<UserList />} />
          <Route path="logs" element={<LogList />} />
          <Route path="discovery" element={<DiscoveryPage />} />
          <Route path="profile" element={<ProfilePage />} />
          <Route path="settings" element={<Settings />} />
        </Route>
        
        {/* 未匹配的路由：已登录去 dashboard，未登录去 login */}
        <Route 
          path="*" 
          element={
            isAuthenticated ? (
              <Navigate to="/dashboard" replace />
            ) : (
              <Navigate to="/login" replace />
            )
          } 
        />
      </Routes>
    </Suspense>
  );
}

export default App;
