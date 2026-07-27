import React from 'react';
import { Checkbox } from '../ui/checkbox';
import { Label } from '../ui/label';

import { useRegisterStore } from '../../lib/stores/useRegisterStore';
import { historyDataGroups } from '../../lib/config/groups';
import { Popover, PopoverTrigger, PopoverContent } from '../ui/popover';
import { Info } from 'lucide-react';
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
              <Label htmlFor={`register-${id}`} className="flex-1 text-sm">
                <div className="flex items-center gap-2">
                  <span className="font-medium">{resolvedRegister?.name || register.name}</span>
                  {showTooltips && (resolvedRegister?.description || register.description) && (
                    <Popover>
                      <PopoverTrigger asChild>
                        <button
                          type="button"
                          className="text-muted-foreground hover:text-foreground hover:bg-muted/50 transition-colors rounded-sm p-1"
                          aria-label={`Info about ${resolvedRegister?.name || register.name}`}
                        >
                          <Info className="h-3.5 w-3.5" />
                        </button>
                      </PopoverTrigger>
                      <PopoverContent className="w-72 max-w-[300px]" align="start" sideOffset={8}>
                        <div className="whitespace-normal break-words">
                          <p className="text-sm font-medium">{resolvedRegister?.name || register.name}</p>
                          <p className="text-sm">{resolvedRegister?.description || register.description}</p>
                          {(resolvedRegister?.unit || register.unit) && (
                            <p className="text-xs text-muted-foreground">
                              Unit: {resolvedRegister?.unit || register.unit}
                            </p>
                          )}
                        </div>
                      </PopoverContent>
                    </Popover>
                  )}
                </div>
                {resolvedRegister?.unit || register.unit ? (
                  <span className="text-xs text-muted-foreground mt-1 block">
                    {resolvedRegister?.unit || register.unit}
                  </span>
                ) : null}
              </Label>
            </div>
          );
        })}
      </div>
    </div>
  );
}
