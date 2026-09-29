import type { SolisStatusDecoded, RegisterMetadata, RegisterValue } from '../../types';

// Check if a status should show an alert (non-standard status).
// Shared by StatusDisplay (Status page) and the flow diagram's inverter node
// so both surfaces agree on what counts as an alert.
export function isAlertStatus(register: RegisterMetadata | undefined, value: RegisterValue | undefined): boolean {
  if (!register || !value) return false;

  const numericValue = typeof value.value === 'number' ? value.value : undefined;

  // Numeric values in status groups - never alert
  if (numericValue !== undefined && register.format) {
    return false;
  }

  // If it has statusDecoded, check the specific status
  if (value.statusDecoded !== undefined && value.statusDecoded !== null) {
    if (Array.isArray(value.statusDecoded)) {
      // For arrays, check if this is a status register that should alert on non-empty
      if (register.id === 'operating_status') {
        // For operating status, check if any element is NOT "Normal operation"
        const hasOnlyNormal = value.statusDecoded.every((s: string) => s.toLowerCase().trim() === 'normal operation');
        return !hasOnlyNormal;
      }
      // For fault status registers, alert if array is not empty OR if it contains non-zero/fault items
      // Check if all items are "0" or "No faults" or "Normal operation" - if so, don't alert
      const hasRealFaults = value.statusDecoded.some((s: string) => {
        const item = String(s).toLowerCase().trim();
        return item !== '0' && item !== 'no faults' && item !== '' && item !== 'normal operation' && !item.includes('normal');
      });
      return hasRealFaults;
    } else if (typeof value.statusDecoded === 'object') {
      const statusObj = value.statusDecoded as SolisStatusDecoded;
      const statusName = (statusObj.name?.toLowerCase() || '').trim();

      // Solis Status - alert if NOT "Generating"
      if (register.id === 'solis_status') {
        return statusName !== 'generating';
      }

      // Operating Status - alert if NOT "Normal operation"
      if (register.id === 'operating_status') {
        return statusName !== 'normal operation';
      }

      // Grid Fault Status - alert if it contains fault/error
      if (register.id === 'grid_fault_1') {
        return statusName !== '' && statusName !== '0' && statusName !== 'no faults';
      }

      // Any other status with alert keywords
      const alertKeywords = ['fault', 'error', 'alarm', 'warning', 'fail', 'off', 'overvoltage', 'undervoltage'];
      return alertKeywords.some(kw => statusName.includes(kw));
    }
  }

  // For numeric values without decoded status (like fault registers showing 0)
  if (numericValue !== undefined && !value.statusDecoded) {
    // If the value is 0, it means no fault - don't alert
    if (numericValue === 0) {
      return false;
    }
    // If it's a fault register and the value is non-zero, alert
    if (register.category === 'status' && numericValue !== 0) {
      return true;
    }
  }

  // Default: don't alert
  return false;
}
