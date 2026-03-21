import { App as AntdApp, ConfigProvider, theme } from 'antd';
import type { PropsWithChildren } from 'react';
import { AuthProvider } from '../shared/auth';

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <ConfigProvider
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: '#d55f28',
          colorInfo: '#19636d',
          colorSuccess: '#2f8f62',
          colorWarning: '#bb7b16',
          colorError: '#bf3f36',
          colorBgBase: '#f5eee2',
          colorBorderSecondary: '#dccfb8',
          borderRadius: 18,
          fontFamily: '"Space Grotesk", "IBM Plex Sans", "Avenir Next", "PingFang SC", sans-serif',
        },
      }}
    >
      <AntdApp>
        <AuthProvider>{children}</AuthProvider>
      </AntdApp>
    </ConfigProvider>
  );
}
