import React from 'react';

/**
 * Inverter status display for the center of the flow diagram
 * Shows solis_status and operating_status with a colored dot
 * Positioned at (0, 0) with Inverter text at y=24, status at y=44, etc.
 */
interface InverterStatusProps {
  status: string;
  operatingStatus: string;
  alert: boolean;
  showIcon?: boolean;
}

/**
 * SVG Inverter Icon - outline only for consistent appearance
 * Larger icon for better visibility
 */
function SvgInverterIcon({ color }: { color: string }): React.ReactElement {
  return (
    <g transform="translate(-18, -18)">
      <rect x={6} y={6} width={12} height={18} rx={2} fill="none" stroke={color} strokeWidth={2.5} />
      <path d="M12 2v5M12 20v3" stroke={color} strokeWidth={2.5} strokeLinecap="round" fill="none" />
      <path d="M8 12h2.5M13.5 12h2.5" stroke={color} strokeWidth={2.5} strokeLinecap="round" fill="none" />
      <path d="M10 16h4" stroke={color} strokeWidth={2.5} strokeLinecap="round" fill="none" />
    </g>
  );
}

/**
 * Get the status color based on alert state
 */
function getStatusColor(alert: boolean): string {
  if (alert) return 'var(--color-flow-status-error)';
  return 'var(--color-flow-status-ok)';
}

export function InverterStatus({ 
  status, 
  operatingStatus,
  alert,
  showIcon = true 
}: InverterStatusProps): React.ReactElement {
  const statusColor = getStatusColor(alert);
  
  return (
    <g>
      {/* Inverter icon at top center (0, -30) - only on desktop */}
      {showIcon && (
        <g transform="translate(0, -30)">
          <SvgInverterIcon color="var(--color-primary)" />
        </g>
      )}
      
      {/* "Inverter" label at y=22 (below icon) */}
      <text 
        x={0} 
        y={22} 
        textAnchor="middle" 
        fill="var(--color-flow-inv-title)" 
        style={{
          fontSize: 18, 
          fontWeight: 700,
          transition: 'opacity 0.5s ease'
        }}
      >
        Inverter
      </text>
      
      {/* Status dot + solis_status at y=50 */}
      <text 
        x={0} 
        y={50} 
        textAnchor="middle" 
        fill={statusColor}
        style={{
          fontSize: 14, 
          fontWeight: 600,
          transition: 'opacity 0.5s ease'
        }}
      >
        ● Solis: {status}
      </text>
      
      {/* Operating status at y=70 */}
      <text 
        x={0} 
        y={70} 
        textAnchor="middle" 
        fill="var(--color-flow-inv-sub)" 
        style={{
          fontSize: 12,
          transition: 'opacity 0.5s ease'
        }}
      >
        Operating: {operatingStatus}
      </text>
    </g>
  );
}
