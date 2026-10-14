import * as Dialog from "@radix-ui/react-dialog";
import { X, ArrowRight, Leaf, RefreshCw } from "lucide-react";
import { Link } from "react-router-dom";
import { useRef, type ReactNode } from "react";
import s from "../styles/App.module.css";

export function Brand() {
  return (
    <Link to="/" className={s.brand}>
      <span className={s.brandIcon}>
        <Leaf size={21} />
      </span>
      <span>
        todo<span className={s.brandLight}> / notificator</span>
      </span>
    </Link>
  );
}
export function Button({
  children,
  quiet = false,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { quiet?: boolean }) {
  return (
    <button
      {...props}
      className={`${quiet ? s.secondary : s.button} ${props.className || ""}`}
    >
      {children}
    </button>
  );
}
export function State({
  title,
  children,
  retry,
  loading = false,
}: {
  title: string;
  children?: ReactNode;
  retry?: () => void;
  loading?: boolean;
}) {
  return (
    <div className={s.state} role={retry ? "alert" : "status"}>
      <span className={s.stateIcon}>
        {loading ? <RefreshCw className={s.spin} /> : <Leaf />}
      </span>
      <h3>{title}</h3>
      {children && <p>{children}</p>}
      {retry && (
        <Button quiet onClick={retry}>
          Попробовать снова <ArrowRight size={16} />
        </Button>
      )}
    </div>
  );
}
export function Sheet({
  open,
  onClose,
  title,
  description,
  children,
  wide = false,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  children: ReactNode;
  wide?: boolean;
}) {
  return (
    <Dialog.Root
      open={open}
      onOpenChange={(value) => {
        if (!value) onClose();
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className={s.overlay} />
        <SheetSurface wide={wide}>
          <div className={s.sheetHeader}>
            <div>
              <Dialog.Title>{title}</Dialog.Title>
              <Dialog.Description className={description ? s.muted : s.srOnly}>
                {description || title}
              </Dialog.Description>
            </div>
            <Dialog.Close className={s.iconButton} aria-label="Закрыть">
              <X size={20} />
            </Dialog.Close>
          </div>
          {children}
        </SheetSurface>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
// This component mounts inside the portal only when it opens, before input autofocus.
function SheetSurface({
  wide,
  children,
}: {
  wide: boolean;
  children: ReactNode;
}) {
  const previousFocus = useRef(
    document.activeElement instanceof HTMLElement
      ? document.activeElement
      : null,
  );
  return (
    <Dialog.Content
      className={wide ? s.modal : s.sheet}
      onCloseAutoFocus={(event) => {
        event.preventDefault();
        if (previousFocus.current?.isConnected) previousFocus.current.focus();
      }}
    >
      {children}
    </Dialog.Content>
  );
}
export function Heading({
  eyebrow,
  title,
  children,
  action,
}: {
  eyebrow?: string;
  title: string;
  children?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <header className={s.heading}>
      <div>
        {eyebrow && <div className={s.eyebrow}>{eyebrow}</div>}
        <h1>{title}</h1>
        {children && <p>{children}</p>}
      </div>
      {action}
    </header>
  );
}
