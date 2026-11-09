"use client";

import { Check, ChevronDown } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import styles from "./task-select.module.css";

export type TaskSelectOption = {
  value: string;
  label: string;
};

export function TaskSelect({
  label,
  value,
  options,
  onChange,
  placeholder,
  name,
  disabled = false,
}: {
  label: string;
  value: string;
  options: TaskSelectOption[];
  onChange: (value: string) => void;
  placeholder: string;
  name?: string;
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [opensAbove, setOpensAbove] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const containerRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const selectedOption = options.find((option) => option.value === value);

  function showMenu() {
    if (disabled) return;
    const rect = triggerRef.current?.getBoundingClientRect();
    setOpensAbove(Boolean(rect && window.innerHeight - rect.bottom < 260 && rect.top > 260));
    setActiveIndex(Math.max(0, options.findIndex((option) => option.value === value)));
    setOpen(true);
  }

  function closeMenu(returnFocus = false) {
    setOpen(false);
    if (returnFocus) triggerRef.current?.focus();
  }

  function moveFocus(nextIndex: number) {
    const clampedIndex = Math.max(0, Math.min(options.length - 1, nextIndex));
    setActiveIndex(clampedIndex);
    optionRefs.current[clampedIndex]?.focus();
  }

  useEffect(() => {
    if (!open) return;
    function handleOutsideClick(event: PointerEvent) {
      if (event.target instanceof Node && !containerRef.current?.contains(event.target)) {
        closeMenu();
      }
    }
    document.addEventListener("pointerdown", handleOutsideClick);
    return () => document.removeEventListener("pointerdown", handleOutsideClick);
  }, [open]);

  function handleOptionKeyDown(event: React.KeyboardEvent<HTMLButtonElement>, index: number) {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      moveFocus((index + 1) % options.length);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      moveFocus((index - 1 + options.length) % options.length);
    } else if (event.key === "Home") {
      event.preventDefault();
      moveFocus(0);
    } else if (event.key === "End") {
      event.preventDefault();
      moveFocus(options.length - 1);
    } else if (event.key === "Escape") {
      event.preventDefault();
      closeMenu(true);
    } else if (event.key === "Tab") {
      closeMenu();
    }
  }

  return (
    <div ref={containerRef} className={styles.root}>
      {name ? <input type="hidden" name={name} value={value} /> : null}
      <button
        ref={triggerRef}
        type="button"
        className={`form-select ${styles.trigger}`}
        aria-label={label}
        aria-haspopup="listbox"
        aria-expanded={open}
        disabled={disabled}
        onClick={() => open ? closeMenu() : showMenu()}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            if (!open) showMenu();
            else moveFocus(activeIndex + (event.key === "ArrowDown" ? 1 : -1));
          }
        }}
      >
        <span className={selectedOption ? "" : styles.placeholder}>{selectedOption?.label ?? placeholder}</span>
        <ChevronDown size={16} aria-hidden="true" className={`${styles.chevron}${open ? ` ${styles.chevronOpen}` : ""}`} />
      </button>
      {open ? (
        <div
          className={`${styles.menu}${opensAbove ? ` ${styles.menuAbove}` : ""}`}
          role="listbox"
          aria-label={label}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.preventDefault();
              closeMenu(true);
            }
          }}
        >
          {options.map((option, index) => (
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
              onClick={() => {
                onChange(option.value);
                closeMenu(true);
              }}
            >
              <span>{option.label}</span>
              {value === option.value ? <Check size={15} aria-hidden="true" /> : null}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
