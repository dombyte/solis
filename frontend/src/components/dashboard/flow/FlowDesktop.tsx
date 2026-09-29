import React from 'react';
import { Factory, Home, ShieldCheck, Sun } from 'lucide-react';
import type { FlowViewModel } from './model';
import { buildFlowSummary, getBackupSubStatus, getBatterySubStatus, getGridSubStatus } from './model';
import { FlowEdge } from './FlowEdge';
import { TileNode, TILE_H } from './TileNode';
import { getBatteryIcon } from './batteryIcon';
import { InverterStatus } from './InverterStatus';

/**
 * Desktop flow diagram with node tiles + inverter box
 */
interface FlowDesktopProps {
  viewModel: FlowViewModel;
}

// Geometry: SVG 1000x800
const SVG_WIDTH = 1000;
const SVG_HEIGHT = 800;

// Inverter box (center)
const C = { x: 500, y: 340 };
const INV = {
  x: C.x - 130,
  y: C.y - 90,
  w: 260,
  h: 180,
};

// Tile centers; the side tiles' centers line up over the edge risers
const PV = { x: 180, y: 90 };
const GRID = { x: 820, y: 90 };
const BAT = { x: 180, y: 590 };
const HH = { x: 820, y: 590 };
const BK = { x: 500, y: 730 };  // Below the inverter

// Half tile height: edges start/end on the tile border
const HALF_H = TILE_H / 2;

// Mid Y for edge calculations
const MID_Y = C.y;

/**
 * Build edge path for vertical edges
 */
function buildVEdge(nx: number, ny: number, side: number): string {
  const ex = side < 0 ? INV.x : INV.x + INV.w;
  const level = ny < MID_Y ? INV.y + 44 : INV.y + INV.h - 44;
  const dir = ny < level ? 1 : -1;
  const cr = 20;
  const cx2 = nx + (side < 0 ? cr : -cr);
  
  return `M ${nx} ${ny} L ${nx} ${level - dir * cr} Q ${nx} ${level} ${cx2} ${level} L ${ex} ${level}`;
}

/**
 * Build edge path for backup (from inverter bottom to backup)
 */
function buildBkEdge(bx: number, by: number): string {
  // BK is centered below inverter, so path is straight down from inverter bottom to BK
  return `M ${C.x} ${INV.y + INV.h} L ${bx} ${INV.y + INV.h} L ${bx} ${by - HALF_H}`;
}

export function FlowDesktop({ viewModel }: FlowDesktopProps): React.ReactElement {
  const { nodes, edges, inverter, stale } = viewModel;
  // Dim once: when the whole diagram is stale the SVG is dimmed, so nodes must not dim
  // again on top of it (0.35 x 0.35 made them nearly invisible, review FE-L3).
  const own = <T extends { stale: boolean }>(n: T): T => (stale ? { ...n, stale: false } : n);

  // Edge paths
  const edgePaths = {
    pv_to_inverter: buildVEdge(PV.x, PV.y + HALF_H, -1),
    grid_to_inverter: buildVEdge(GRID.x, GRID.y + HALF_H, 1),
    battery_to_inverter: buildVEdge(BAT.x, BAT.y - HALF_H, -1),
    inverter_to_household: buildVEdge(HH.x, HH.y - HALF_H, 1),
    inverter_to_backup: buildBkEdge(BK.x, BK.y),
  };

  const soc = nodes.battery.soc === undefined ? '–' : `${Math.round(nodes.battery.soc)}%`;

  return (
    <div className="w-full">
    <svg
      viewBox={`0 0 ${SVG_WIDTH} ${SVG_HEIGHT}`}
      className="w-full h-auto"
      style={{
        opacity: stale ? 0.35 : 1,
        transition: 'opacity 0.5s ease'
      }}
    >
      {/* Edges */}
      <FlowEdge 
        d={edgePaths.pv_to_inverter}
        active={edges.pv_to_inverter.active}
        reverse={edges.pv_to_inverter.reverse}
        color="var(--color-flow-pv)"
      />
      <FlowEdge 
        d={edgePaths.grid_to_inverter}
        active={edges.grid_to_inverter.active}
        reverse={edges.grid_to_inverter.reverse}
        color="var(--color-flow-grid)"
      />
      <FlowEdge 
        d={edgePaths.battery_to_inverter}
        active={edges.battery_to_inverter.active}
        reverse={edges.battery_to_inverter.reverse}
        color="var(--color-flow-batt)"
      />
      <FlowEdge 
        d={edgePaths.inverter_to_household}
        active={edges.inverter_to_household.active}
        reverse={edges.inverter_to_household.reverse}
        color="var(--color-flow-hh)"
      />
      <FlowEdge 
        d={edgePaths.inverter_to_backup}
        active={edges.inverter_to_backup.active}
        reverse={false} // Always flows FROM inverter to backup
        color="var(--color-flow-bk)"
      />

      {/* Nodes */}
      <TileNode node={own(nodes.pv)} cx={PV.x} cy={PV.y} icon={Sun} />
      <TileNode
        node={own(nodes.grid)}
        cx={GRID.x}
        cy={GRID.y}
        icon={Factory}
        sub={getGridSubStatus(nodes.grid.value)}
      />
      <TileNode
        node={own(nodes.battery)}
        cx={BAT.x}
        cy={BAT.y}
        icon={getBatteryIcon(nodes.battery.soc, nodes.battery.value)}
        title={`Battery · ${soc}`}
        sub={getBatterySubStatus(nodes.battery.value)}
      />
      <TileNode node={own(nodes.household)} cx={HH.x} cy={HH.y} icon={Home} />
      <TileNode
        node={own(nodes.backup)}
        cx={BK.x}
        cy={BK.y}
        icon={ShieldCheck}
        sub={getBackupSubStatus(nodes.backup.value)}
      />

      {/* Inverter hub at center */}
      <InverterStatus
        x={INV.x}
        y={INV.y}
        width={INV.w}
        height={INV.h}
        status={inverter.status}
        operatingStatus={inverter.operatingStatus}
        alert={inverter.alert}
      />
    </svg>
    <div className="sr-only" role="note">
      {buildFlowSummary(viewModel)}
    </div>
    </div>
  );
}
