import type { SolisStatusDecoded, RegisterMetadata, RegisterValue } from '../../types';

/**
 * How a status register reads:
 * - ok: producing / no fault bit set
 * - notice: a normal state that is not producing (waiting at night, standby, limited
 *   operation): amber in the diagram, never a badge or a notification
 * - fault: a fault code or fault bit: red, counted in the nav badge, banner and
 *   notifications
 * - unknown: no value
 */
export type StatusLevel = 'ok' | 'notice' | 'fault' | 'unknown';

// solis_status codes below 0x1000 are operating states (internal/solis/status_tables.go);
// anything else, including an unknown code, is a fault.
const SOLIS_STATES = new Set([
  'waiting', 'openrun', 'softrun', 'generating', 'standby', 'standbysynch', 'gridtoload', 'normal',
]);
const SOLIS_PRODUCING = new Set(['generating', 'normal']);

// operating_status bits that are normal states; any other set bit (fault/warning bits,
// unknown bits) is a fault.
const OPERATING_NORMAL = 'normal operation';
const OPERATING_NOTICE = new Set([
  'initializing', 'controlled turn-off', 'stand-by',
  'limited operation (temp/freq)', 'limited operation (external)',
]);

function norm(s: unknown): string {
  return String(s).toLowerCase().trim();
}

function solisLevel(decoded: unknown): StatusLevel {
  const name = norm((decoded as Partial<SolisStatusDecoded> | null)?.name ?? '');
  if (name === '') return 'unknown';
  if (SOLIS_PRODUCING.has(name)) return 'ok';
  return SOLIS_STATES.has(name) ? 'notice' : 'fault';
}

function operatingLevel(bits: string[]): StatusLevel {
  const set = bits.map(norm).filter(b => b !== OPERATING_NORMAL);
  if (set.some(b => !OPERATING_NOTICE.has(b))) return 'fault';
  return set.length > 0 ? 'notice' : 'ok';
}

/** The level of a status register's decoded value (see StatusLevel). */
function statusLevelOf(id: string, decoded: unknown): StatusLevel {
  if (decoded === null || decoded === undefined) return 'unknown';
  if (id === 'solis_status') return solisLevel(decoded);
  if (!Array.isArray(decoded)) return 'unknown';
  if (id === 'operating_status') return operatingLevel(decoded);
  // Fault bitmasks: any set bit is a fault.
  return decoded.length > 0 ? 'fault' : 'ok';
}

/** The level of a register's current value. */
export function statusLevel(register: RegisterMetadata | undefined, value: RegisterValue | undefined): StatusLevel {
  if (!register || !value) return 'unknown';
  return statusLevelOf(register.id, value.statusDecoded);
}

/**
 * Display text of a status register: the status name, the active fault bits, "No faults"
 * for a bitmask with no bit set, else the plain value ("-" without one).
 */
export function statusText(decoded: unknown, value?: unknown): string {
  if (Array.isArray(decoded)) return decoded.length > 0 ? decoded.join(', ') : 'No faults';
  if (decoded !== null && typeof decoded === 'object') {
    return (decoded as { name?: string }).name || JSON.stringify(decoded);
  }
  if (decoded !== null && decoded !== undefined) return String(decoded);
  return value === null || value === undefined ? '-' : String(value);
}
