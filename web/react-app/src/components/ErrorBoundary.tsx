import React, { Component, ErrorInfo, ReactNode } from 'react';
import { Result, Button } from 'antd';
import { RefreshCw } from 'lucide-react';

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
  onError?: (error: Error, errorInfo: ErrorInfo) => void;
}

interface State {
  hasError: boolean;
  error?: Error;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('ErrorBoundary caught an error:', error, errorInfo);
    
    if (this.props.onError) {
      this.props.onError(error, errorInfo);
    }
  }

  handleRetry = () => {
    this.setState({ hasError: false, error: undefined });
  };

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) {
        return this.props.fallback;
      }

      return (
        <Result
          status="error"
          title="Something went wrong"
          subTitle="An unexpected error occurred while rendering this component."
          extra={[
            <Button 
              type="primary" 
              icon={<RefreshCw size={14} strokeWidth={1.7} />} 
              onClick={this.handleRetry}
              key="retry"
            >
              Try Again
            </Button>,
            <Button 
              onClick={() => window.location.reload()}
              key="reload"
            >
              Reload Page
            </Button>
          ]}
        >
          {process.env.NODE_ENV === 'development' && this.state.error && (
            <div style={{ 
              textAlign: 'left', 
              marginTop: 16, 
              padding: 16, 
              backgroundColor: '#f5f5f5',
              borderRadius: 4,
              fontSize: 12,
              fontFamily: 'monospace'
            }}>
              <strong>Error Details:</strong>
              <pre>{this.state.error.stack}</pre>
            </div>
          )}
        </Result>
      );
    }

    return this.props.children;
  }
}

export default ErrorBoundary;