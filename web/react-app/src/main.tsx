import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { ConfigProvider, theme } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import App from './App';
import './index.css';

// Superview immersive dark theme tokens (obsidian glass)
const designTokens = {
  colorPrimary: '#38bdf8',
  colorInfo: '#38bdf8',
  colorSuccess: '#34d399',
  colorWarning: '#fbbf24',
  colorError: '#fb7185',
  colorBgBase: '#0a0c14',
  colorBgContainer: 'rgba(255,255,255,0.04)',
  colorBgElevated: '#141828',
  colorText: '#f4f6ff',
  colorTextSecondary: '#a5b0d0',
  colorTextTertiary: '#6b7694',
  colorBorder: 'rgba(255,255,255,0.08)',
  colorBorderSecondary: 'rgba(255,255,255,0.06)',
  borderRadius: 10,
  fontSize: 13.5,
  fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
};

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <BrowserRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <ConfigProvider
        locale={zhCN}
        theme={{ algorithm: theme.darkAlgorithm, token: designTokens }}
      >
        <App />
      </ConfigProvider>
    </BrowserRouter>
  </React.StrictMode>
);