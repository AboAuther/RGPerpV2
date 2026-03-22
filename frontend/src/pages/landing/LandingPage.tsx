import { Button, Typography } from 'antd';
import { useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  siBinance,
  siChainlink,
  siCoinbase,
  siEthereum,
  siGitkraken,
  siOptimism,
  siPolygon,
  siSolana,
} from 'simple-icons';
import BlobCursor from '../../components/reactbits/BlobCursor';
import MagicRings from '../../components/reactbits/MagicRings';
import { BrandLogo } from '../../shared/components';
import GlitchText from '../../components/landing/GlitchText';
import ShinyText from '../../components/landing/ShinyText';
import VariableProximityText from '../../components/landing/VariableProximityText';
import './LandingPage.css';

type LogoItem = {
  name: string;
  hex: string;
  path: string;
};

type FeatureItem = {
  title: string;
  description: string;
};

type StatItem = {
  label: string;
  value: string;
};

const logos: LogoItem[] = [
  { name: siBinance.title, hex: siBinance.hex, path: siBinance.path },
  { name: siCoinbase.title, hex: siCoinbase.hex, path: siCoinbase.path },
  { name: siEthereum.title, hex: siEthereum.hex, path: siEthereum.path },
  { name: siSolana.title, hex: siSolana.hex, path: siSolana.path },
  { name: siPolygon.title, hex: siPolygon.hex, path: siPolygon.path },
  { name: siOptimism.title, hex: siOptimism.hex, path: siOptimism.path },
  { name: siChainlink.title, hex: siChainlink.hex, path: siChainlink.path },
  { name: siGitkraken.title, hex: siGitkraken.hex, path: siGitkraken.path },
];

const stats: StatItem[] = [
  { label: 'Custody + Wallet', value: 'Multi-Chain' },
  { label: 'Ledger & Audit', value: 'Traceable' },
  { label: 'Risk Engine', value: 'Real-Time' },
  { label: 'Ops Console', value: 'Integrated' },
];

const features: FeatureItem[] = [
  {
    title: '安全托管与资金可追溯',
    description: '覆盖充值、提现、冻结、确认与失败退回，全流程围绕统一账本和状态机组织，让每一笔资金动作都可追踪、可审计。',
  },
  {
    title: '风险优先的交易体验',
    description: '从 reduce-only、风险率预警到清算状态提示，所有关键风险信息都在交易界面中前置呈现，优先保护账户安全。',
  },
  {
    title: '全链路 Explorer 与审计视图',
    description: '链上交易、账本事件、订单、成交、仓位与资金动作能够统一检索和关联，为排障、对账与审计提供可验证依据。',
  },
  {
    title: '面向运营与风控的一体化控制台',
    description: '用户前台与管理后台共享同一套产品语义，覆盖资产概览、钱包流程、风险治理、提现审核和清算处置。',
  },
];

export function LandingPage() {
  const navigate = useNavigate();
  const logoTrack = useMemo(() => [...logos, ...logos], []);

  return (
    <div className="landing-root">
      <div className="landing-rings-layer" aria-hidden>
        <MagicRings
          color="#ff38f5"
          colorTwo="#4ee7ff"
          speed={0.85}
          ringCount={8}
          attenuation={10}
          lineThickness={3}
          baseRadius={0.18}
          radiusStep={0.09}
          scaleRate={0.16}
          opacity={1}
          noiseAmount={0.008}
          rotation={0}
          ringGap={1.22}
          fadeIn={0.66}
          fadeOut={0.78}
          followMouse={false}
          mouseInfluence={0}
          hoverScale={1}
          parallax={0}
          clickBurst={false}
        />
      </div>
      <div className="landing-rings-glow-layer" aria-hidden>
        <MagicRings
          color="#ff54f7"
          colorTwo="#77ffe1"
          speed={0.82}
          ringCount={8}
          attenuation={7.5}
          lineThickness={4.6}
          baseRadius={0.18}
          radiusStep={0.09}
          scaleRate={0.16}
          opacity={0.85}
          blur={18}
          noiseAmount={0}
          rotation={0}
          ringGap={1.22}
          fadeIn={0.66}
          fadeOut={0.78}
          followMouse={false}
          mouseInfluence={0}
          hoverScale={1}
          parallax={0}
          clickBurst={false}
        />
      </div>
      <div className="landing-rings-vignette" aria-hidden />
      <BlobCursor
        fillColor="#8b5cff"
        trailCount={3}
        sizes={[48, 92, 64]}
        innerSizes={[16, 28, 18]}
        innerColor="rgba(255,255,255,0.72)"
        opacities={[0.46, 0.22, 0.16]}
        shadowColor="rgba(120, 78, 255, 0.24)"
        shadowBlur={18}
        shadowOffsetX={0}
        shadowOffsetY={0}
        filterStdDeviation={26}
        zIndex={7}
      />
      <div className="landing-noise" aria-hidden />

      <main className="landing-main">
        <header className="landing-topbar">
          <button type="button" className="landing-brand-button" onClick={() => navigate('/')}>
            <BrandLogo size={48} />
            <ShinyText text="RG Perp" className="landing-brand-text" />
          </button>
          <Button className="landing-docs-top-btn" size="large" type="text" onClick={() => navigate('/trade')}>
            进入平台
          </Button>
        </header>

        <section className="landing-hero">
          <div className="landing-copy">
            <Typography.Title level={1} className="landing-title">
              <GlitchText text="Custody, Trade," className="landing-glitch-line" />
              <br />
              <GlitchText text="Risk &" className="landing-glitch-line" />
              <br />
              <GlitchText text="Audit" className="landing-glitch-line" />
            </Typography.Title>
            <Typography.Paragraph className="landing-subtitle">
              <VariableProximityText
                text="A production-grade perpetual platform built around secure custody, unified ledger truth, realtime risk controls, and operator-ready visibility."
                className="landing-variable-text"
              />
            </Typography.Paragraph>

            <div className="landing-actions">
              <Button className="landing-launch-btn" size="large" onClick={() => navigate('/trade')}>
                Launch Platform
              </Button>
            </div>
          </div>
        </section>

        <section className="landing-logos-wrap" aria-label="ecosystem">
          <Typography.Text className="landing-logos-caption">Market context & ecosystem references</Typography.Text>
          <div className="landing-logos-track">
            {logoTrack.map((item, idx) => (
              <div key={`${item.name}-${idx}`} className="landing-logo-item">
                <span className="landing-logo-mark" aria-hidden>
                  <svg viewBox="0 0 24 24" role="img">
                    <path d={item.path} fill={`#${item.hex}`} />
                  </svg>
                </span>
                <span>{item.name}</span>
              </div>
            ))}
          </div>
        </section>

        <section className="landing-stats-section">
          {stats.map((item) => (
            <article key={item.label} className="landing-stat-panel">
              <span className="landing-stat-label">{item.label}</span>
              <strong className="landing-stat-value">{item.value}</strong>
            </article>
          ))}
        </section>

        <section className="landing-feature-section">
          <div className="landing-section-copy">
            <Typography.Text className="landing-section-tag">Why RGPerp</Typography.Text>
            <Typography.Title level={2} className="landing-section-title">
              面向真实市场的永续基础设施
            </Typography.Title>
            <Typography.Paragraph className="landing-section-subtitle">
              从多链托管与统一账本，到风险引擎、清算处理、Explorer 与运营后台，RGPerp 将交易、资金与审计能力收敛到同一条可信链路中。
            </Typography.Paragraph>
          </div>

          <div className="landing-feature-grid">
            {features.map((item) => (
              <article key={item.title} className="landing-feature-card">
                <span className="landing-feature-kicker">Platform Capability</span>
                <Typography.Title level={4} className="landing-feature-title">
                  {item.title}
                </Typography.Title>
                <Typography.Paragraph className="landing-feature-description">
                  {item.description}
                </Typography.Paragraph>
              </article>
            ))}
          </div>
        </section>
      </main>
    </div>
  );
}
