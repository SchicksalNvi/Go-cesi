import { useNavigate } from 'react-router-dom';
import { ArrowLeft, Home, SearchX } from 'lucide-react';
import { useStore } from '@/store';

// 品牌化 404 页:未匹配路由展示,提供返回路径
const NotFound: React.FC = () => {
  const navigate = useNavigate();
  const { t } = useStore();

  return (
    <div
      style={{
        minHeight: '70vh',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 22,
        padding: '0 24px',
        textAlign: 'center',
      }}
    >
      <div
        style={{
          width: 72,
          height: 72,
          borderRadius: 20,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          background: 'var(--grad-soft)',
          border: '1px solid rgba(129,140,248,0.25)',
          color: 'var(--acc-violet)',
        }}
      >
        <SearchX size={30} strokeWidth={1.7} />
      </div>

      <div className="display" style={{ fontSize: 64, lineHeight: 1, letterSpacing: '-0.03em' }}>
        <span className="gradient-text">404</span>
      </div>

      <div className="hero-kicker" style={{ fontSize: 11 }}>
        {t.notFound.kicker}
      </div>

      <p style={{ color: 'var(--text-mid)', fontSize: 14, maxWidth: 380, lineHeight: 1.6, margin: 0 }}>
        {t.notFound.message}
      </p>

      <div style={{ display: 'flex', gap: 12, marginTop: 8 }}>
        <button className="sv-btn sv-btn-primary" style={{ height: 40, padding: '0 22px' }} onClick={() => navigate(-1)}>
          <ArrowLeft size={15} strokeWidth={2} />
          {t.notFound.back}
        </button>
        <button className="sv-btn" style={{ height: 40, padding: '0 22px' }} onClick={() => navigate('/dashboard')}>
          <Home size={15} strokeWidth={2} />
          {t.notFound.home}
        </button>
      </div>
    </div>
  );
};

export default NotFound;