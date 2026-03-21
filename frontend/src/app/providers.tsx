import { App as AntdApp, ConfigProvider, theme } from 'antd';
import type { PropsWithChildren } from 'react';
import { AuthProvider } from '../shared/auth';

export function AppProviders({ children }: PropsWithChildren) {
  return (
    <ConfigProvider
      theme={{
        algorithm: theme.darkAlgorithm,
        token: {
          colorPrimary: '#20c9b5',
          colorInfo: '#5eb2ff',
          colorSuccess: '#2ec9b0',
          colorWarning: '#fbbf24',
          colorError: '#ff6b6b',
          colorBgBase: '#08131b',
          colorBgContainer: '#0d1b24',
          colorText: '#edf6ff',
          colorTextSecondary: '#8ca3b8',
          colorBorderSecondary: '#17303d',
          borderRadius: 16,
          fontFamily: '"IBM Plex Sans", "PingFang SC", "Noto Sans SC", sans-serif',
          fontFamilyCode: '"IBM Plex Mono", monospace',
        },
        components: {
          Layout: {
            headerBg: 'rgba(10, 24, 33, 0.88)',
            bodyBg: 'transparent',
            footerBg: 'transparent',
          },
          Card: {
            colorBgContainer: 'rgba(12, 27, 36, 0.88)',
            boxShadowTertiary: '0 16px 40px rgba(0,0,0,0.28)',
          },
          Table: {
            headerBg: '#102431',
            rowHoverBg: '#102634',
          },
          Menu: {
            darkItemBg: 'transparent',
            darkSubMenuItemBg: 'rgba(8, 19, 31, 0.92)',
            darkItemSelectedBg: 'rgba(32, 201, 181, 0.12)',
          },
          Alert: {
            withDescriptionIconSize: 18,
          },
        },
      }}
    >
      <AntdApp>
        <AuthProvider>{children}</AuthProvider>
      </AntdApp>
    </ConfigProvider>
  );
}
