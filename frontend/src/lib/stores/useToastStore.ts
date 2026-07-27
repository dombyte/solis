import { create } from 'zustand';

type ToastVariant = 'default' | 'destructive' | 'success' | 'warning';

interface ToastData {
  id: string;
  title: string;
  description?: string;
  variant: ToastVariant;
  duration?: number;
  createdAt: number;
}

interface ToastState {
  toasts: ToastData[];
  toast: (data: Omit<ToastData, 'id' | 'createdAt'>) => string;
  dismiss: (id: string) => void;
}

let toastId = 0;

export const useToastStore = create<ToastState>((set) => ({
  toasts: [],
  
  toast: (data) => {
    const id = String(++toastId);
    const toastData: ToastData = {
      ...data,
      id,
      createdAt: Date.now(),
      duration: data.duration ?? 4000,
    };
    
    set((state) => ({ toasts: [...state.toasts, toastData] }));
    
    // Auto-dismiss after duration
    const duration = toastData.duration ?? 4000;
    if (duration > 0) {
      setTimeout(() => {
        set((state) => ({
          toasts: state.toasts.filter((t) => t.id !== id)
        }));
      }, duration);
    }
    
    return id;
  },
  
  dismiss: (id) => {
    set((state) => ({
      toasts: state.toasts.filter((t) => t.id !== id)
    }));
  },
}));
