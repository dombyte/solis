import React from 'react';
import { Sun, Factory, Home, ShieldCheck } from 'lucide-react';
import type { FlowViewModel } from './model';
import { getBackupSubStatus, getGridSubStatus, getBatterySubStatus, buildFlowSummary } from './model';
import { ANIMATION_DURATION } from './FlowEdge';
import { getBatteryIcon } from './batteryIcon';

/**
 * Mobile flow diagram with card list and flow rail
 * Responsive width like system status card
 */
interface FlowMobileProps {
  viewModel: FlowViewModel;
}

const CARD_H = 62;

export function FlowMobile({ viewModel }: FlowMobileProps): React.ReactElement {
  const { nodes, inverter, stale } = viewModel;
  const opacity = stale ? 0.4 : 1;

  // Card configuration
  const cards = [
    {
      key: 'pv' as const,
      name: 'PV',
      value: nodes.pv.displayValue,
      sub: '',
      color: 'var(--color-flow-pv)',
      active: nodes.pv.active,
      stale: nodes.pv.stale,
      Icon: Sun,
      connector: {
        active: nodes.pv.active,
        reverse: true,
        color: 'var(--color-flow-pv)'
      },
    },
    {
      key: 'grid' as const,
      name: 'Grid',
      value: nodes.grid.displayValue,
      sub: getGridSubStatus(nodes.grid.value),
      color: 'var(--color-flow-grid)',
      active: nodes.grid.active,
      stale: nodes.grid.stale,
      Icon: Factory,
      connector: {
        active: nodes.grid.active,
        reverse: nodes.grid.value !== null ? nodes.grid.value < 0 : false,
        color: 'var(--color-flow-grid)'
      },
    },
    {
      key: 'battery' as const,
      name: `Battery · ${nodes.battery.soc === undefined ? '–' : `${Math.round(nodes.battery.soc)}%`}`,
      value: nodes.battery.displayValue,
      sub: getBatterySubStatus(nodes.battery.value),
      color: 'var(--color-flow-batt)',
      active: nodes.battery.active,
      stale: nodes.battery.stale,
      Icon: getBatteryIcon(nodes.battery.soc, nodes.battery.value),
      connector: {
        active: nodes.battery.active,
        reverse: nodes.battery.value !== null ? nodes.battery.value < 0 : false,
        color: 'var(--color-flow-batt)'
      },
    },
    {
      key: 'household' as const,
      name: 'Household',
      value: nodes.household.displayValue,
      sub: '',
      color: 'var(--color-flow-hh)',
      active: nodes.household.active,
      stale: nodes.household.stale,
      Icon: Home,
      connector: {
        active: nodes.household.active,
        reverse: false,
        color: 'var(--color-flow-hh)'
      },
    },
    {
      key: 'backup' as const,
      name: 'Backup',
      value: nodes.backup.displayValue,
      sub: getBackupSubStatus(nodes.backup.value),
      color: 'var(--color-flow-bk)',
      active: nodes.backup.active,
      stale: nodes.backup.stale,
      Icon: ShieldCheck,
      connector: {
        active: nodes.backup.active,
        reverse: false,
        color: 'var(--color-flow-bk)'
      },
    },
  ];

  // Inverter header
  const statusColor = inverter.alert ? 'var(--color-flow-status-error)' : 'var(--color-flow-status-ok)';

  return (
    <div 
      style={{
        opacity,
        transition: 'opacity 0.5s ease'
      }} 
      className="w-full min-w-0"  // Responsive width like DataCard
    >
      <div>
        {/* Cards */}
        <div className="flex flex-col gap-2">
          {/* Inverter header card - full width, no battery icon */}
          <div 
            className="rounded-xl px-3.5 py-2.5 text-center flex flex-wrap items-center justify-center gap-x-2 gap-y-0.5"
            style={{ 
              background: 'var(--color-flow-inv-bg)',
              border: '1.5px solid var(--color-border)'
            }}
          >
            <span style={{ color: 'var(--color-flow-inv-title)', fontSize: 14, fontWeight: 700 }}>Inverter</span>
            <span style={{ color: statusColor, fontSize: 12, fontWeight: 600 }}>● Solis: {inverter.status}</span>
            <span style={{ color: 'var(--color-flow-inv-sub)', fontSize: 11 }}>Operating: {inverter.operatingStatus}</span>
          </div>

          {/* Node cards with the rail on the left. The rail lives inside this column, so
              it always starts below the inverter header (keeping the gap-2 spacing) however
              many lines the header wraps to. */}
          <div className="relative flex flex-col gap-2">
            <div
              aria-hidden="true"
              className="absolute top-0 bottom-0 rounded-full pointer-events-none"
              style={{ left: 17, width: 10, background: 'var(--color-flow-track)' }}
            />
            {cards.map((card) => (
              <div key={card.key} className="relative" style={{ height: CARD_H }}>
                {/* Connector from rail to card */}
                <svg 
                  className="absolute left-0" 
                  style={{ top: 0, width: '100%', height: CARD_H }}
                >
                  <RailConnector
                    y={CARD_H / 2}
                    active={card.connector.active}
                    reverse={card.connector.reverse}
                    color={card.connector.color}
                  />
                </svg>

                {/* Card */}
                <div
                  className="flex items-center justify-between rounded-xl px-3 h-full ml-auto"
                  style={{
                    background: 'var(--color-card)',
                    border: `1.5px solid ${card.color}`,
                    width: '70%',
                    // the container already dims a stale diagram: dim once (FE-L3)
                    opacity: card.stale && !stale ? 0.35 : 1,
                    transition: 'opacity 0.5s ease'
                  }}
                  role="img"
                  aria-label={`${card.name}: ${card.stale ? 'no data' : card.value}${card.sub ? ` (${card.sub})` : ''}`}
                >
                  <div className="flex items-center gap-3">
                    <card.Icon size={22} color={card.color} strokeWidth={2.25} aria-hidden="true" />
                    <div>
                      <div style={{ 
                        color: 'var(--color-muted-foreground)', 
                        fontSize: 12, 
                        fontWeight: 500 
                      }}>
                        {card.name}
                      </div>
                      <div style={{ 
                        color: 'var(--color-foreground)', 
                        fontSize: 15, 
                        fontWeight: 700 
                      }}>
                        {card.value}
                      </div>
                      {card.sub && (
                        <div style={{ 
                          color: 'var(--color-muted-foreground)', 
                          fontSize: 11 
                        }}>
                          {card.sub}
                        </div>
                      )}
                    </div>
                  </div>

                  {/* Flow badge */}
                  <FlowBadge color={card.color} active={card.active} />
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
      <div className="sr-only" role="note">
        {buildFlowSummary(viewModel)}
      </div>
    </div>
  );
}

/**
 * Flow badge: pulsing dot
 */
function FlowBadge({ color, active }: { color: string; active: boolean }): React.ReactElement {
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', justifyContent: 'center', width: 20 }}>
      <span
        style={{
          width: 8,
          height: 8,
          borderRadius: 999,
          background: color,
          opacity: active ? 1 : 0.25,
          transition: 'opacity 0.5s ease',
          boxShadow: active ? `0 0 6px ${color}` : 'none',
        }}
      />
    </span>
  );
}

/**
 * Animated rail connector for mobile
 */
function RailConnector({ y, active, reverse, color }: { y: number; active: boolean; reverse: boolean; color: string }): React.ReactElement {
  if (!active) {
    return (
      <g>
        <line 
          x1={28} y1={y} x2="30%" y2={y} 
          stroke="var(--color-flow-track)" 
          strokeWidth={5} 
          strokeLinecap="round"
        />
      </g>
    );
  }

  const animationDirection = reverse ? 'reverse' : 'normal';

  return (
    <g>
      <line 
        x1={28} y1={y} x2="30%" y2={y} 
        stroke="var(--color-flow-track)" 
        strokeWidth={5} 
        strokeLinecap="round"
      />
      
      <line
        x1={28}
        y1={y}
        x2="30%"
        y2={y}
        stroke={color}
        strokeWidth={4}
        strokeLinecap="round"
        strokeDasharray="7 8"
        strokeDashoffset={0}
        opacity={1}
        style={{
          animation: `flowRail ${ANIMATION_DURATION} linear infinite`,
          animationDirection,
          transition: 'opacity 0.5s ease',
        }}
      />
    </g>
  );
}
