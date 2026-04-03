import { useToast } from '../../store/useToast';
import { CheckCircle2, AlertCircle, Info, X } from 'lucide-react';

export function ToastContainer() {
  const { toasts, removeToast } = useToast();

  return (
    <div className='fixed bottom-6 right-6 z-[100] flex flex-col gap-3 pointer-events-none'>
      {toasts.map((t) => (
        <div
          key={t.id}
          className='pointer-events-auto flex items-start gap-4 p-4 min-w-[320px] max-w-sm rounded-2xl shadow-xl border bg-white'
        >
          <div className='mt-0.5 shrink-0'>
            {t.type === 'success' && <CheckCircle2 className='text-emerald-500' size={24} />}
            {t.type === 'error' && <AlertCircle className='text-red-500' size={24} />}
            {t.type === 'info' && <Info className='text-blue-500' size={24} />}
          </div>
          <div className='flex-1 break-words'>
            <h3 className='font-bold text-gray-900'>{t.message}</h3>
          </div>
          <button
            onClick={() => removeToast(t.id)}
            className='text-gray-400 hover:text-gray-600 hover:bg-gray-100 p-1 rounded-lg transition-colors shrink-0'
          >
            <X size={16} />
          </button>
        </div>
      ))}
    </div>
  );
}