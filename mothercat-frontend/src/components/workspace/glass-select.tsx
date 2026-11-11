"use client";

import { Check, ChevronDown } from "lucide-react";
import { createPortal } from "react-dom";
import { useCallback, useEffect, useId, useRef, useState, type KeyboardEvent } from "react";

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

type MenuPosition = {
  top: number;
  left: number;
  width: number;
  maxHeight: number;
};

const viewportGutter = 12;
const preferredMenuWidth = 352;
const maximumMenuHeight = 520;

function getMenuPosition(trigger: HTMLButtonElement, optionCount: number): MenuPosition {
  const rect = trigger.getBoundingClientRect();
  const viewportWidth = document.documentElement.clientWidth || window.innerWidth;
  const viewportHeight = window.innerHeight;
  const availableBelow = Math.max(0, viewportHeight - rect.bottom - viewportGutter * 2);
  const availableAbove = Math.max(0, rect.top - viewportGutter * 2);
  const desiredHeight = Math.min(Math.max(optionCount, 1) * 52 + 16, maximumMenuHeight);
  const opensAbove = availableBelow < Math.min(desiredHeight, 180) && availableAbove > availableBelow;
  const maxHeight = Math.max(64, Math.min(desiredHeight, opensAbove ? availableAbove : availableBelow));
  const availableWidth = Math.max(1, viewportWidth - viewportGutter * 2);
  const width = Math.min(Math.max(rect.width, preferredMenuWidth), availableWidth);
  const left = Math.max(viewportGutter, Math.min(rect.left, viewportWidth - width - viewportGutter));
  const top = opensAbove
    ? Math.max(viewportGutter, rect.top - maxHeight - 6)
    : Math.min(viewportHeight - maxHeight - viewportGutter, rect.bottom + 6);
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
  const [activeIndex, setActiveIndex] = useState(0);
  const [position, setPosition] = useState<MenuPosition | null>(null);
  const [portalTarget, setPortalTarget] = useState<HTMLElement | null>(null);
  const [hasOverflow, setHasOverflow] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const typeaheadRef = useRef({ value: "", timeout: 0 });
  const id = useId();
  const listboxId = `${id}-listbox`;
  const selectedOption = options.find((option) => option.value === value);

  const close = useCallback((returnFocus = false) => {
    setOpen(false);
    setPortalTarget(null);
    if (returnFocus) requestAnimationFrame(() => triggerRef.current?.focus());
  }, []);

  const openMenu = useCallback(() => {
    if (disabled) return;
    const index = options.findIndex((option) => option.value === value);
    setActiveIndex(index >= 0 ? index : 0);
    if (triggerRef.current) {
      setPosition(getMenuPosition(triggerRef.current, options.length));
      setPortalTarget(getPortalTarget(triggerRef.current));
    }
    setOpen(true);
  }, [disabled, options, value]);

  useEffect(() => {
    if (!open) return;

    function updatePosition() {
      if (triggerRef.current) setPosition(getMenuPosition(triggerRef.current, options.length));
      if (panelRef.current) setHasOverflow(panelRef.current.scrollHeight > panelRef.current.clientHeight + 1);
    }

    function handleOutsidePointer(event: PointerEvent) {
      if (!(event.target instanceof Node)) return;
      if (triggerRef.current?.contains(event.target) || panelRef.current?.contains(event.target)) return;
      close();
    }

    function handleDocumentKeyDown(event: globalThis.KeyboardEvent) {
      if (event.key === "Escape") {
        event.preventDefault();
        close(true);
      }
    }

    updatePosition();
    document.addEventListener("pointerdown", handleOutsidePointer);
    document.addEventListener("keydown", handleDocumentKeyDown);
    window.addEventListener("resize", updatePosition);
    window.addEventListener("scroll", updatePosition, true);
    const resizeObserver = panelRef.current && typeof ResizeObserver !== "undefined"
      ? new ResizeObserver(updatePosition)
      : null;
    if (panelRef.current) resizeObserver?.observe(panelRef.current);

    const focusedOption = optionRefs.current[activeIndex];
    focusedOption?.focus({ preventScroll: true });

    return () => {
      document.removeEventListener("pointerdown", handleOutsidePointer);
      document.removeEventListener("keydown", handleDocumentKeyDown);
      window.removeEventListener("resize", updatePosition);
      window.removeEventListener("scroll", updatePosition, true);
      resizeObserver?.disconnect();
    };
  }, [activeIndex, close, open, options.length]);

  function choose(option: GlassSelectOption) {
    onChange(option.value);
    close(true);
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
    } else if ((event.key === "Enter" || event.key === " ") && !open) {
      event.preventDefault();
      openMenu();
    } else if (event.key === "Escape" && open) {
      event.preventDefault();
      close(true);
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
    } else if (event.key === "Escape") {
      event.preventDefault();
      close(true);
    } else if (event.key === "Tab") {
      close();
    } else if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      event.preventDefault();
      window.clearTimeout(typeaheadRef.current.timeout);
      typeaheadRef.current.value += event.key.toLocaleLowerCase();
      typeaheadRef.current.timeout = window.setTimeout(() => {
        typeaheadRef.current.value = "";
      }, 500);
      const startIndex = index + 1;
      const orderedOptions = [
        ...options.slice(startIndex),
        ...options.slice(0, startIndex),
      ];
      const match = orderedOptions.find((option) =>
        option.label.toLocaleLowerCase().startsWith(typeaheadRef.current.value),
      );
      if (match) {
        const matchIndex = options.indexOf(match);
        setActiveIndex(matchIndex);
        optionRefs.current[matchIndex]?.focus();
      }
    }
  }

  const menu = open && position && portalTarget
    ? createPortal(
        <div
          ref={panelRef}
          id={listboxId}
          className={`workspace-modal-panel workspace-select-panel no-scrollbar${hasOverflow ? " is-scrollable" : ""}`}
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
          {options.length === 0 ? (
            <p className="workspace-select-empty" role="status">{emptyMessage}</p>
          ) : options.map((option, index) => (
            <button
              key={option.value}
              ref={(element) => { optionRefs.current[index] = element; }}
              type="button"
              role="option"
              aria-selected={value === option.value}
              tabIndex={activeIndex === index ? 0 : -1}
              className="workspace-modal-link workspace-select-option"
              data-active={value === option.value}
              onFocus={() => setActiveIndex(index)}
              onKeyDown={(event) => handleOptionKeyDown(event, index)}
              onClick={() => choose(option)}
            >
              <span className="workspace-select-label">{option.label}</span>
              {value === option.value ? <Check size={16} aria-hidden="true" /> : null}
            </button>
          ))}
        </div>,
        portalTarget,
      )
    : null;

  return (
    <>
      {name ? <input type="hidden" name={name} value={value} /> : null}
      <button
        ref={triggerRef}
        type="button"
        className="form-select workspace-select-trigger"
        aria-label={label}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listboxId : undefined}
        title={selectedOption?.label ?? placeholder}
        disabled={disabled}
        onClick={() => open ? close() : openMenu()}
        onKeyDown={handleTriggerKeyDown}
      >
        <span className={`workspace-select-value${selectedOption ? "" : " is-placeholder"}`}>
          {selectedOption?.label ?? placeholder}
        </span>
        <ChevronDown size={16} aria-hidden="true" className={`workspace-select-chevron${open ? " is-open" : ""}`} />
      </button>
      {menu}
    </>
  );
}

function getPortalTarget(trigger: HTMLButtonElement | null): HTMLElement {
  const dialog = trigger?.closest("dialog");
  return dialog instanceof HTMLDialogElement && dialog.open ? dialog : document.body;
}
