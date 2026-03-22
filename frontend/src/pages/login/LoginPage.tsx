import { App as AntdApp, Button, Card, Space, Steps, Typography } from 'antd';
import { useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import ShinyText from '../../components/landing/ShinyText';
import VariableProximityText from '../../components/landing/VariableProximityText';
import { api } from '../../shared/api';
import { useAuth } from '../../shared/auth';
import { BrandLogo, ErrorAlert } from '../../shared/components';

const { Title, Paragraph, Text } = Typography;

type LoginPhase = 'idle' | 'connecting_wallet' | 'requesting_challenge' | 'awaiting_signature' | 'verifying' | 'success' | 'error';

export function LoginPage() {
  const { message } = AntdApp.useApp();
  const { signIn } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();

  const [phase, setPhase] = useState<LoginPhase>('idle');
  const [connectedAddress, setConnectedAddress] = useState<string>('');
  const [connectedChainId, setConnectedChainId] = useState<number | null>(null);
  const [error, setError] = useState<unknown>(null);

  const redirectTo = (location.state as { from?: string } | null)?.from || '/portfolio';

  async function handleWalletLogin() {
    if (!window.ethereum) {
      setError(new Error('未检测到 MetaMask 或兼容钱包'));
      return;
    }

    try {
      setError(null);
      setPhase('connecting_wallet');

      const accounts = (await window.ethereum.request({ method: 'eth_requestAccounts' })) as string[];
      const address = accounts[0];
      if (!address) {
        throw new Error('未获取到钱包地址');
      }

      const chainHex = (await window.ethereum.request({ method: 'eth_chainId' })) as string;
      const chainId = Number.parseInt(chainHex, 16);
      if (!Number.isFinite(chainId) || chainId <= 0) {
        throw new Error('当前钱包链 ID 无效');
      }

      setConnectedAddress(address);
      setConnectedChainId(chainId);

      setPhase('requesting_challenge');
      const challenge = await api.auth.challenge(address, chainId);

      setPhase('awaiting_signature');
      const signature = (await window.ethereum.request({
        method: 'personal_sign',
        params: [challenge.message, address],
      })) as string;

      setPhase('verifying');
      const response = await api.auth.login({
        address,
        chainId,
        nonce: challenge.nonce,
        signature,
      });

      signIn({
        accessToken: response.access_token,
        refreshToken: response.refresh_token,
        expiresAt: response.expires_at,
        user: response.user,
      });

      setPhase('success');
      message.success('登录成功');
      navigate(redirectTo, { replace: true });
    } catch (loginError) {
      setPhase('error');
      setError(loginError);
    }
  }

  const stepItems = [
    { title: '连接钱包', description: '读取地址与链 ID' },
    { title: '签发挑战', description: '服务端绑定域名与 nonce' },
    { title: '签名验签', description: '钱包签名后完成会话创建' },
  ];

  const stepCurrent =
    phase === 'idle' || phase === 'connecting_wallet'
      ? 0
      : phase === 'requesting_challenge'
        ? 1
        : 2;

  return (
    <div className="rg-app-page login-page">
      <div className="login-page-header">
        <Space size={14}>
          <BrandLogo size={42} />
          <div>
            <Title level={3} style={{ margin: 0 }}>
              <ShinyText text="RGPerp" className="page-intro-title-text" />
            </Title>
            <Text type="secondary">Production-grade perp console</Text>
          </div>
        </Space>
      </div>
      <div className="login-grid">
        <Card className="hero-card" bordered={false}>
          <Text className="page-intro-eyebrow">Wallet Auth</Text>
          <Title level={1} style={{ maxWidth: 680, marginTop: 0 }}>
            直接通过 MetaMask 一键登录，登录挑战由后端签发并绑定当前链环境。
          </Title>
          <Paragraph className="page-intro-description page-intro-description--proximity" style={{ fontSize: 17, maxWidth: 760 }}>
            <VariableProximityText
              text="登录只保留一条真实路径：连接钱包、请求挑战、签名验签。前端不保存资金真相，不复用 challenge，不再保留手工 nonce 或演示登录分支。"
              className="page-intro-description-text"
            />
          </Paragraph>

          <div className="hero-highlight">
            <div>
              <Text strong>地址来源</Text>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                当前地址与链 ID 只从浏览器钱包读取，不允许手工伪造。
              </Paragraph>
            </div>
            <div>
              <Text strong>挑战消息</Text>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                后端返回完整 message，前端仅负责签名，不自行拼装业务真相。
              </Paragraph>
            </div>
            <div>
              <Text strong>会话保持</Text>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                access token 仅保存在内存，刷新页面后需要重新登录。
              </Paragraph>
            </div>
          </div>
        </Card>

        <Card className="surface-card" bordered={false}>
          <Space direction="vertical" size={20} style={{ width: '100%' }}>
            <div>
              <Title level={3} style={{ marginTop: 0, marginBottom: 8 }}>
                <ShinyText text="MetaMask Login" className="page-intro-title-text" />
              </Title>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                点击一次按钮即可完成连接钱包、请求 challenge、签名和登录。
              </Paragraph>
            </div>

            <Steps
              current={stepCurrent}
              status={phase === 'error' ? 'error' : phase === 'success' ? 'finish' : 'process'}
              items={stepItems}
            />

            <Button
              type="primary"
              size="large"
              onClick={handleWalletLogin}
              loading={phase !== 'idle' && phase !== 'success' && phase !== 'error'}
            >
              连接钱包并登录
            </Button>

            {connectedAddress ? (
              <Card size="small" className="surface-card" bordered={false}>
                <Space direction="vertical" size={4}>
                  <Text strong>当前钱包地址</Text>
                  <Text code>{connectedAddress}</Text>
                  <Text type="secondary">链 ID: {connectedChainId ?? '-'}</Text>
                </Space>
              </Card>
            ) : null}

            <ErrorAlert error={error} />
          </Space>
        </Card>
      </div>
    </div>
  );
}
