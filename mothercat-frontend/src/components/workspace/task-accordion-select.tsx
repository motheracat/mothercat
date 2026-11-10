
"use client";

import {
  Check,
  ChevronDown,
  Search,
} from "lucide-react";

import {
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";

import styles from "./task-accordion-select.module.css";

export type TaskAccordionSelectOption = {
  value: string;
  label: string;
};

type Props = {
  label: string;
  value: string;
  options: TaskAccordionSelectOption[];
  onChange: (value: string) => void;
  placeholder: string;
  emptyMessage?: string;
  name?: string;
  disabled?: boolean;
};

export function TaskAccordionSelect({
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
  const [activeIndex, setActiveIndex] = useState(-1);

  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const optionRefs = useRef<(HTMLButtonElement | null)[]>([]);

  const panelId = useId();
  const listboxId = useId();

  const selectedOption = options.find(
    (option) => option.value === value
  );

  function closeDropdown(restoreFocus = false) {
    setOpen(false);
    setActiveIndex(-1);

    if (restoreFocus) {
      requestAnimationFrame(() => {
        triggerRef.current?.focus();
      });
    }
  }

  function openDropdown() {
    if (disabled) return;

    const selectedIndex = options.findIndex(
      (option) => option.value === value
    );

    setActiveIndex(
      selectedIndex >= 0 ? selectedIndex : options.length ? 0 : -1
    );

    setOpen(true);
  }

  function selectOption(option: TaskAccordionSelectOption) {
    onChange(option.value);
    closeDropdown(true);
  }

  // Close when clicking outside
  useEffect(() => {
    function handleOutsideClick(event: MouseEvent) {
      if (
        rootRef.current &&
        !rootRef.current.contains(event.target as Node)
      ) {
        closeDropdown();
      }
    }

    document.addEventListener("mousedown", handleOutsideClick);

    return () => {
      document.removeEventListener("mousedown", handleOutsideClick);
    };
  }, []);

  // Close if disabled while open
  useEffect(() => {
    if (disabled) {
      setOpen(false);
      setActiveIndex(-1);
    }
  }, [disabled]);

  // Focus active option when keyboard navigation changes index
  useEffect(() => {
    if (open && activeIndex >= 0) {
      optionRefs.current[activeIndex]?.focus();
    }
  }, [open, activeIndex]);

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (disabled) return;

    if (event.key === "Escape" && open) {
      event.preventDefault();
      closeDropdown(true);
      return;
    }

    if (
      !open &&
      (event.key === "Enter" ||
        event.key === " " ||
        event.key === "ArrowDown" ||
        event.key === "ArrowUp")
    ) {
      event.preventDefault();
      openDropdown();
      return;
    }

    if (!open || !options.length) return;

    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActiveIndex((current) =>
        current >= options.length - 1 ? 0 : current + 1
      );
    }

    if (event.key === "ArrowUp") {
      event.preventDefault();
      setActiveIndex((current) =>
        current <= 0 ? options.length - 1 : current - 1
      );
    }

    if (event.key === "Home") {
      event.preventDefault();
      setActiveIndex(0);
    }

    if (event.key === "End") {
      event.preventDefault();
      setActiveIndex(options.length - 1);
    }

    if (event.key === "Enter" && activeIndex >= 0) {
      event.preventDefault();
      selectOption(options[activeIndex]);
    }

    if (event.key === "Tab") {
      closeDropdown();
    }
  }

  return (
    <div
      ref={rootRef}
      className={`${styles.root} ${open ? styles.rootOpen : ""} ${
        disabled ? styles.rootDisabled : ""
      }`}
      onKeyDown={handleKeyDown}
    >
      {name && (
        <input type="hidden" name={name} value={value} />
      )}

      <button
        ref={triggerRef}
        type="button"
        className={styles.trigger}
        aria-label={label}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={panelId}
        disabled={disabled}
        onClick={() => {
          if (open) {
            closeDropdown();
          } else {
            openDropdown();
          }
        }}
      >
        <span className={styles.triggerContent}>
          <span className={styles.label}>{label}</span>

          <span
            className={
              selectedOption
                ? styles.value
                : styles.placeholder
            }
          >
            {selectedOption?.label ?? placeholder}
          </span>
        </span>

        <span className={styles.triggerIcon}>
          <ChevronDown
            size={19}
            strokeWidth={2}
            aria-hidden="true"
          />
        </span>
      </button>

      <div
        id={panelId}
        className={`${styles.panel} ${
          open ? styles.panelOpen : ""
        }`}
        aria-hidden={!open}
        inert={!open}
      >
        <div className={styles.panelHeader}>
          <div>
            <span className={styles.panelTitle}>
              {label}
            </span>
            <span className={styles.panelSubtitle}>
              Select one option
            </span>
          </div>

          <span className={styles.optionCount}>
            {options.length}
          </span>
        </div>

        <div
          id={listboxId}
          className={styles.options}
          role="listbox"
          aria-label={`${label} options`}
        >
          {options.length > 0 ? (
            options.map((option, index) => {
              const selected = value === option.value;
              const active = activeIndex === index;

              return (
                <button
                  key={option.value}
                  ref={(element) => {
                    optionRefs.current[index] = element;
                  }}
                  type="button"
                  role="option"
                  aria-selected={selected}
                  tabIndex={active ? 0 : -1}
                  className={`${styles.option} ${
                    selected ? styles.optionSelected : ""
                  } ${active ? styles.optionActive : ""}`}
                  onMouseEnter={() => setActiveIndex(index)}
                  onClick={() => selectOption(option)}
                >
                  <span className={styles.optionText}>
                    {option.label}
                  </span>

                  <span className={styles.optionCheck}>
                    {selected && (
                      <Check size={17} strokeWidth={2.5} />
                    )}
                  </span>
                </button>
              );
            })
          ) : (
            <div className={styles.emptyState}>
              <span className={styles.emptyIcon}>
                <Search size={20} />
              </span>
              <span className={styles.emptyTitle}>
                Nothing to select
              </span>
              <span className={styles.emptyMessage}>
                {emptyMessage}
              </span>
            </div>
          )}
        </div>

        {options.length > 0 && (
          <div className={styles.panelFooter}>
            <span>Use ↑ ↓ to navigate</span>
            <span>Enter to select</span>
          </div>
        )}
      </div>
    </div>
  );
}
