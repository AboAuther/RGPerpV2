import {
  App as AntdApp,
  Alert,
  Button,
  Card,
  Descriptions,
  Divider,
  Form,
  Input,
  Select,
  Space,
  Steps,
  Typography,
} from 'antd';
import { useMemo, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { api, buildLoginMessage } from '../../shared/api';
import { useAuth } from '../../shared/auth';
import { BrandLogo, ErrorAlert } from '../../shared/components';
import type { NonceResponse } from '../../shared/domain';
import { appConfig } from '../../shared/env';
import { formatDateTime } from '../../shared/format';

const { Title, Paragraph, Text } = Typography;

type LoginPhase =
  | 'idle'
  | 'requesting_nonce'
  | 'awaiting_signature'
  | 'verifying'
  | 'success'
  | 'error';

interface LoginFormValues {
  address: string;
  chainId: number;
  signature?: string;
}

export function LoginPage() {
  const [form] = Form.useForm<LoginFormValues>();
  const { message } = AntdApp.useApp();
  const { signIn } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();

  const [phase, setPhase] = useState<LoginPhase>('idle');
  const [noncePayload, setNoncePayload] = useState<(NonceResponse & { provider: 'mock' | 'http' }) | null>(null);
  const [error, setError] = useState<unknown>(null);

  const walletAvailable = typeof window !== 'undefined' && !!window.ethereum;
  const reviewMockLoginEnabled = appConfig.reviewFeaturesEnabled && appConfig.apiProvider === 'mock';
  const redirectTo = (location.state as { from?: string } | null)?.from || '/portfolio';

  const loginMessage = useMemo(() => {
    if (!noncePayload) {
      return '';
    }
    return buildLoginMessage(noncePayload.domain, noncePayload.chain_id, noncePayload.nonce);
  }, [noncePayload]);

  async function handleConnectWallet() {
    if (!window.ethereum) {
      message.warning('未检测到浏览器钱包，可手动填写地址并在下方粘贴签名。');
      return;
    }

    try {
      const accounts = (await window.ethereum.request({
        method: 'eth_requestAccounts',
      })) as string[];

      const chainHex = (await window.ethereum.request({ method: 'eth_chainId' })) as string;
      const chainId = parseInt(chainHex, 16);

      form.setFieldsValue({
        address: accounts[0],
        chainId,
      });
    } catch (walletError) {
      setError(walletError);
    }
  }

  async function handleRequestNonce() {
    setError(null);
    const values = await form.validateFields(['address', 'chainId']);
    setPhase('requesting_nonce');

    try {
      const response = await api.auth.issueNonce(values.address, values.chainId);
      setNoncePayload(response);
      setPhase('awaiting_signature');
      message.success(`Nonce 已签发，来源: ${response.provider.toUpperCase()}`);
    } catch (requestError) {
      setPhase('error');
      setError(requestError);
    }
  }

  async function handleWalletSignature(address: string): Promise<string> {
    if (!window.ethereum) {
      throw new Error('未检测到钱包扩展，请手动粘贴离线签名结果。');
    }

    const signature = (await window.ethereum.request({
      method: 'personal_sign',
      params: [loginMessage, address],
    })) as string;

    form.setFieldValue('signature', signature);
    return signature;
  }

  async function handleLogin() {
    if (!noncePayload) {
      await handleRequestNonce();
      return;
    }

    setError(null);
    const values = await form.validateFields(['address', 'chainId', 'signature']);
    let signature = values.signature?.trim();

    try {
      setPhase('awaiting_signature');

      if (!signature && walletAvailable) {
        signature = await handleWalletSignature(values.address);
      }

      if (!signature) {
        throw new Error('缺少签名。请使用钱包签名，或粘贴离线签名结果。');
      }

      setPhase('verifying');
      const response = await api.auth.login({
        address: values.address,
        chainId: values.chainId,
        nonce: noncePayload.nonce,
        signature,
      });

      signIn({
        accessToken: response.access_token,
        refreshToken: response.refresh_token,
        expiresAt: response.expires_at,
        user: response.user,
        provider: response.provider,
      });

      setPhase('success');
      message.success('登录成功');
      navigate(redirectTo, { replace: true });
    } catch (loginError) {
      setPhase('error');
      setError(loginError);
    }
  }

  async function handleReviewMockLogin() {
    if (!reviewMockLoginEnabled) {
      return;
    }

    try {
      setError(null);
      const values = await form.validateFields(['address', 'chainId']);
      setPhase('requesting_nonce');
      const challenge = noncePayload || (await api.auth.issueNonce(values.address, values.chainId));
      setNoncePayload(challenge);
      setPhase('verifying');

      const response = await api.auth.login({
        address: values.address,
        chainId: values.chainId,
        nonce: challenge.nonce,
        signature: '0xreview_mock_login',
      });

      signIn({
        accessToken: response.access_token,
        refreshToken: response.refresh_token,
        expiresAt: response.expires_at,
        user: response.user,
        provider: response.provider,
      });

      setPhase('success');
      message.success('Review mock 登录成功');
      navigate(redirectTo, { replace: true });
    } catch (loginError) {
      setPhase('error');
      setError(loginError);
    }
  }

  const stepItems = [
    { title: '请求 Nonce', description: '绑定域名与链环境' },
    { title: '等待签名', description: '用户确认登录用途' },
    { title: '验签登录', description: '服务端验签并创建会话' },
  ];

  const stepCurrent =
    phase === 'idle'
      ? 0
      : phase === 'requesting_nonce'
        ? 0
        : phase === 'awaiting_signature'
          ? 1
          : 2;

  return (
    <div className="rg-app-page login-page">
      <div className="login-page-header">
        <Space size={14}>
          <BrandLogo size={42} />
          <div>
            <Title level={3} style={{ margin: 0 }}>
              RGPerp
            </Title>
            <Text type="secondary">Production-grade perp console</Text>
          </div>
        </Space>
      </div>
      <div className="login-grid">
        <Card className="hero-card" bordered={false}>
          <Text className="page-intro-eyebrow">Milestone 2</Text>
          <Title level={1} style={{ maxWidth: 680, marginTop: 0 }}>
            钱包登录先绑定域名、链 ID 和一次性 nonce，再进入资金与账户视图。
          </Title>
          <Paragraph style={{ fontSize: 17, maxWidth: 760 }}>
            这个实现遵循当前规范：前端不持有资金真相，不把 nonce 复用，不把 access token 持久化到不安全存储。
            关键登录、账户和钱包路径默认直连真实接口；只有显式 `review/dev + mock provider` 才允许进入隔离的演示流。
          </Paragraph>

          <div className="hero-highlight">
            <div>
              <Text strong>登录状态</Text>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                请求 nonce、中间签名、服务端验签、成功或失败分态展示。
              </Paragraph>
            </div>
            <div>
              <Text strong>会话保持</Text>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                真实 token 只保留在内存；仅 mock 会话快照允许保存在当前 tab。
              </Paragraph>
            </div>
            <div>
              <Text strong>安全边界</Text>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                路由守卫只做 UX。最终权限和 RBAC 仍以后端为准。
              </Paragraph>
            </div>
            <div>
              <Text strong>当前 Provider</Text>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                `VITE_API_PROVIDER={appConfig.apiProvider}`。登录与资金关键路径不会在 `auto` 下静默回退 mock。
              </Paragraph>
            </div>
          </div>
        </Card>

        <Card className="surface-card" bordered={false}>
          <Space direction="vertical" size={20} style={{ width: '100%' }}>
            <div>
              <Title level={3} style={{ marginTop: 0, marginBottom: 8 }}>
                Sign-In Console
              </Title>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                先请求一次性 nonce，再签名登录。`ACCEPTED` 不等于已授权所有操作，后续仍要以后端权限校验为准。
              </Paragraph>
            </div>

            <Steps
              current={stepCurrent}
              status={phase === 'error' ? 'error' : phase === 'success' ? 'finish' : 'process'}
              items={stepItems}
            />

            <Form form={form} layout="vertical" initialValues={{ chainId: appConfig.supportedChains[0]?.id }}>
              <Form.Item label="钱包地址" name="address" rules={[{ required: true, message: '请输入 EVM 地址' }]}>
                <Input placeholder="0x..." />
              </Form.Item>

              <Form.Item label="链 ID" name="chainId" rules={[{ required: true, message: '请选择链环境' }]}>
                <Select
                  options={appConfig.supportedChains.map((chain) => ({
                    label: `${chain.name} (${chain.id})`,
                    value: chain.id,
                  }))}
                />
              </Form.Item>

              <Form.Item label="签名结果（可选）" name="signature">
                <Input.TextArea
                  rows={4}
                  placeholder={
                    walletAvailable
                      ? '如不想用浏览器钱包，可粘贴离线签名结果'
                      : '未检测到浏览器钱包，请粘贴离线签名结果'
                  }
                />
              </Form.Item>

              <Space wrap>
                <Button onClick={handleConnectWallet}>连接钱包</Button>
                <Button onClick={handleRequestNonce} loading={phase === 'requesting_nonce'}>
                  请求 Nonce
                </Button>
                <Button type="primary" onClick={handleLogin} loading={phase === 'verifying'}>
                  签名并登录
                </Button>
                {reviewMockLoginEnabled ? (
                  <Button onClick={handleReviewMockLogin} loading={phase === 'requesting_nonce' || phase === 'verifying'}>
                    Review Mock 登录
                  </Button>
                ) : null}
              </Space>
            </Form>

            {noncePayload ? (
              <>
                <Divider style={{ margin: 0 }} />
                <Descriptions size="small" column={1} title="登录挑战">
                  <Descriptions.Item label="Domain">{noncePayload.domain}</Descriptions.Item>
                  <Descriptions.Item label="Chain ID">{noncePayload.chain_id}</Descriptions.Item>
                  <Descriptions.Item label="Expires At">{formatDateTime(noncePayload.expires_at)}</Descriptions.Item>
                  <Descriptions.Item label="Provider">{noncePayload.provider.toUpperCase()}</Descriptions.Item>
                </Descriptions>
                <Input.TextArea value={loginMessage} autoSize={{ minRows: 3, maxRows: 6 }} readOnly />
              </>
            ) : null}

            {phase === 'success' ? (
              <Alert showIcon type="success" message="登录成功，正在进入账户概览。" />
            ) : (
              <Alert
                showIcon
                type="info"
                message="登录提示"
                description={
                  reviewMockLoginEnabled
                    ? '当前是显式 review mock 环境。真实签名登录与 review 演示登录已分离，避免假签名混入真实登录流。'
                    : '真实登录必须使用有效签名。若未检测到浏览器钱包，请手动粘贴离线签名结果。'
                }
              />
            )}

            <ErrorAlert error={error} />
          </Space>
        </Card>
      </div>
    </div>
  );
}
