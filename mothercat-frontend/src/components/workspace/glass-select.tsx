"use client";

import { Check, ChevronDown } from "lucide-react";
import { createPortal } from "react-dom";
import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react";

import styles from "./glass-select.module.css";

export type GlassSelectOption = {
  value: string;
  label: string;
};

type Props = {
  label: string;
  value: string;
  options: GlassSelectOption[];
  onChange: (value: string) => void;
  placeholder: string;
  emptyMessage?: string;
  name?: string;
  disabled?: boolean;
};

type PopoverPosition = {
  top: number;
  left: number;
  width: number;
  maxHeight: number;
};

const viewportGutter = 12;

function getPopoverPosition(trigger: HTMLButtonElement, optionCount: number): PopoverPosition {
  const rect = trigger.getBoundingClientRect();
  const availableBelow = window.innerHeight - rect.bottom - viewportGutter * 2;
  const availableAbove = rect.top - viewportGutter * 2;
  const desiredHeight = Math.min(optionCount * 43 + 14, 320);
  const opensAbove = availableBelow < Math.min(desiredHeight, 180) && availableAbove > availableBelow;
  const maxHeight = Math.max(100, Math.min(desiredHeight, opensAbove ? availableAbove : availableBelow));
  const width = Math.min(rect.width, window.innerWidth - viewportGutter * 2);
  const left = Math.max(viewportGutter, Math.min(rect.left, window.innerWidth - width - viewportGutter));
  const top = opensAbove
    ? Math.max(viewportGutter, rect.top - maxHeight - 7)
    : Math.min(window.innerHeight - maxHeight - viewportGutter, rect.bottom + 7);
  return { top, left, width, maxHeight };
}

export function GlassSelect({
  label,
  value,
  options,
  onChange,
  placeholder,
  emptyMessage = "No options available.",
  name,
  disabled = false,
}: Props) {
  const [open, setOpen] = useState(false);
  const [isMobile, setIsMobile] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const [position, setPosition] = useState<PopoverPosition | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const selectedOption = options.find((option) => option.value === value);
  const panelTitleId = `glass-select-title-${name ?? label.toLowerCase().replace(/[^a-z0-9]+/g, "-")}`;

  const close = useCallback((returnFocus = true) => {
    setOpen(false);
    if (returnFocus) requestAnimationFrame(() => triggerRef.current?.focus());
  }, []);

  const openMenu = useCallback(() => {
    if (disabled) return;
    const mobile = window.matchMedia("(max-width: 767px)").matches;
    setIsMobile(mobile);
    const index = options.findIndex((option) => option.value === value);
    setActiveIndex(index >= 0 ? index : 0);
    if (!mobile && triggerRef.current) {
      setPosition(getPopoverPosition(triggerRef.current, options.length));
    }
    setOpen(true);
  }, [disabled, options, value]);

  useEffect(() => {
    if (!open || !isMobile) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previousOverflow;
    };
  }, [open, isMobile]);

  useEffect(() => {
    if (!open) return;

    const mediaQuery = window.matchMedia("(max-width: 767px)");
    function updatePosition() {
      setIsMobile(mediaQuery.matches);
      if (!mediaQuery.matches && triggerRef.current) {
        setPosition(getPopoverPosition(triggerRef.current, options.length));
      }
    }
    function handleOutsidePointer(event: PointerEvent) {
      if (!(event.target instanceof Node)) return;
      if (triggerRef.current?.contains(event.target) || panelRef.current?.contains(event.target)) return;
      close();
    }
    function handleKeyDown(event: globalThis.KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        close();
      }
    }

    updatePosition();
    document.addEventListener("pointerdown", handleOutsidePointer);
    document.addEventListener("keydown", handleKeyDown);
    window.addEventListener("resize", updatePosition);
    window.addEventListener("scroll", updatePosition, true);
    mediaQuery.addEventListener("change", updatePosition);
    const focusedOption = optionRefs.current[activeIndex];
    focusedOption?.focus({ preventScroll: true });
    return () => {
      document.removeEventListener("pointerdown", handleOutsidePointer);
      document.removeEventListener("keydown", handleKeyDown);
      window.removeEventListener("resize", updatePosition);
      window.removeEventListener("scroll", updatePosition, true);
      mediaQuery.removeEventListener("change", updatePosition);
    };
  }, [open, isMobile, options.length, activeIndex, close]);

  function choose(option: GlassSelectOption) {
    onChange(option.value);
    close();
  }

  function moveFocus(index: number) {
    if (options.length === 0) return;
    const nextIndex = (index + options.length) % options.length;
    setActiveIndex(nextIndex);
    optionRefs.current[nextIndex]?.focus();
  }

  function handleTriggerKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      if (!open) openMenu();
      else moveFocus(activeIndex + (event.key === "ArrowDown" ? 1 : -1));
    } else if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      if (!open) openMenu();
    } else if (event.key === "Escape" && open) {
      event.preventDefault();
      close();
    }
  }

  function handleOptionKeyDown(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      moveFocus(index + 1);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      moveFocus(index - 1);
    } else if (event.key === "Home") {
      event.preventDefault();
      moveFocus(0);
    } else if (event.key === "End") {
      event.preventDefault();
      moveFocus(options.length - 1);
    } else if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      choose(options[index]);
    } else if (event.key === "Escape" || event.key === "Tab") {
      event.preventDefault();
      close();
    }
  }

  const panel = open && typeof document !== "undefined"
    ? isMobile
      ? createPortal(
          <>
            <button type="button" className={styles.backdrop} aria-label={`Close ${label} options`} onClick={() => close()} />
            <div
              ref={panelRef}
              className={styles.sheet}
              role="dialog"
              aria-modal="true"
              aria-labelledby={panelTitleId}
            >
              <span className={styles.dragHandle} aria-hidden="true" />
              <h2 id={panelTitleId} className={styles.sheetTitle}>{label}</h2>
              <div className={styles.sheetList} role="listbox" aria-label={label}>
                {options.length === 0 ? <p role="status" className={styles.emptyMessage}>{emptyMessage}</p> : options.map((option, index) => (
                  <button
                    key={option.value}
                    ref={(element) => { optionRefs.current[index] = element; }}
                    type="button"
                    role="option"
                    aria-selected={value === option.value}
                    tabIndex={activeIndex === index ? 0 : -1}
                    className={`${styles.option}${value === option.value ? ` ${styles.optionSelected}` : ""}`}
                    onFocus={() => setActiveIndex(index)}
                    onKeyDown={(event) => handleOptionKeyDown(event, index)}
                    onClick={() => choose(option)}
                  >
                    <span className={styles.optionLabel}>{option.label}</span>
                    {value === option.value ? <Check size={17} aria-hidden="true" /> : null}
                  </button>
                ))}
              </div>
            </div>
          </>,
          getPortalTarget(triggerRef.current),
        )
      : position
        ? createPortal(
            <div
              ref={panelRef}
              className={styles.popover}
              role="listbox"
              aria-label={label}
              style={{
                position: "fixed",
                zIndex: 1300,
                top: position.top,
                left: position.left,
                width: position.width,
                maxHeight: position.maxHeight,
              }}
            >
              {options.length === 0 ? <p role="status" className={styles.emptyMessage}>{emptyMessage}</p> : options.map((option, index) => (
                <button
                  key={option.value}
                  ref={(element) => { optionRefs.current[index] = element; }}
                  type="button"
                  role="option"
                  aria-selected={value === option.value}
                  tabIndex={activeIndex === index ? 0 : -1}
                  className={`${styles.option}${value === option.value ? ` ${styles.optionSelected}` : ""}`}
                  onFocus={() => setActiveIndex(index)}
                  onKeyDown={(event) => handleOptionKeyDown(event, index)}
                  onClick={() => choose(option)}
                >
                  <span className={styles.optionLabel}>{option.label}</span>
                  {value === option.value ? <Check size={15} aria-hidden="true" /> : null}
                </button>
              ))}
            </div>,
            getPortalTarget(triggerRef.current),
          )
        : null
    : null;

  return (
    <>
      {name ? <input type="hidden" name={name} value={value} /> : null}
      <button
        ref={triggerRef}
        type="button"
        className={styles.trigger}
        aria-label={label}
        aria-haspopup="listbox"
        aria-expanded={open}
        disabled={disabled}
        onClick={() => open ? close() : openMenu()}
        onKeyDown={handleTriggerKeyDown}
      >
        <span className={`${styles.triggerText}${selectedOption ? "" : ` ${styles.placeholder}`}`}>
          {selectedOption?.label ?? placeholder}
        </span>
        <ChevronDown size={16} aria-hidden="true" className={`${styles.chevron}${open ? ` ${styles.chevronOpen}` : ""}`} />
      </button>
      {panel}
    </>
  );
}

function getPortalTarget(trigger: HTMLButtonElement | null): HTMLElement {
  const dialog = trigger?.closest("dialog");
  return dialog instanceof HTMLDialogElement && dialog.open ? dialog : document.body;
}
