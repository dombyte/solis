import React from 'react';
import { Checkbox } from '../ui/checkbox';
import { Label } from '../ui/label';

import { useRegisterStore } from '../../lib/stores/useRegisterStore';
import { historyDataGroups } from '../../lib/config/groups';
import { InfoPopover } from '../ui/info-popover';
import type { Period } from '../../types';

interface RegisterSelectorProps {
  selectedIds: string[];
  onToggle: (id: string) => void;
  period: Period;
  className?: string;
}

export function RegisterSelector({
  selectedIds,
  onToggle,
  period,
  className = ''
}: RegisterSelectorProps): React.ReactElement {
  const getRegisterById = useRegisterStore(state => state.getRegisterById);
  const getResolvedRegisterById = useRegisterStore(state => state.getResolvedRegisterById);
  const showTooltips = true;
  
  // Get available register IDs for this period
  const availableIds = historyDataGroups[period] || [];
  
  // Get only registers that have metadata and are available for this period
  const validIds = availableIds.filter(id => {
    const register = getRegisterById(id);
    return register !== undefined;
  });

  if (validIds.length === 0) {
    return <div className={className}>No registers available for this period</div>;
  }

  return (
    <div className={`space-y-2 ${className}`}>
      <div className="space-y-1">
        {validIds.map((id) => {
          const resolvedRegister = getResolvedRegisterById(id);
          const register = resolvedRegister || getRegisterById(id);
          const isSelected = selectedIds.includes(id);
          
          if (!register) return null;
          const name = resolvedRegister?.name || register.name;
          const description = resolvedRegister?.description || register.description;
          const unit = resolvedRegister?.unit || register.unit;
          
          return (
            <div 
              key={id} 
              className="flex items-center gap-3 p-2 rounded-lg hover:bg-muted/50 transition-colors touch-target min-h-[48px]"
            >
              <Checkbox
                id={`register-${id}`}
                checked={isSelected}
                onCheckedChange={() => onToggle(id)}
                className="h-5 w-5"
              />
              <div className="flex flex-1 items-start gap-2">
                <Label htmlFor={`register-${id}`} className="min-w-0 text-sm">
                  <span className="font-medium">{name}</span>
                  {unit ? (
                    <span className="text-xs text-muted-foreground mt-1 block">{unit}</span>
                  ) : null}
                </Label>
                {showTooltips && description && (
                  <InfoPopover label={name} align="start">
                    <p className="font-medium">{name}</p>
                    <p>{description}</p>
                    {unit && <p className="text-xs text-muted-foreground">Unit: {unit}</p>}
                  </InfoPopover>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
