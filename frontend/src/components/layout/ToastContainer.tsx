import { useToastStore } from '../../lib/stores/useToastStore';
import {
  Toast,
  ToastTitle,
  ToastDescription,
  ToastClose,
  ToastViewport,
} from '../ui/toast';

export function ToastContainer() {
  const { toasts, dismiss } = useToastStore();

  return (
    <ToastViewport>
      {toasts.map((toast) => (
        <Toast
          key={toast.id}
          variant={toast.variant}
          duration={toast.duration ?? 4000}
          onOpenChange={(open) => {
            if (!open) dismiss(toast.id);
          }}
        >
          <div className="grid gap-1">
            {toast.title && <ToastTitle>{toast.title}</ToastTitle>}
            {toast.description && (
              <ToastDescription>{toast.description}</ToastDescription>
            )}
          </div>
          <ToastClose onClick={() => dismiss(toast.id)} />
        </Toast>
      ))}
    </ToastViewport>
  );
}
