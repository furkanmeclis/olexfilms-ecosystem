"use client";

import { useEffect, useMemo, useState } from "react";

/** Matches FormSection scroll-mt-24 + sticky app header. */
const ACTIVATION_OFFSET = 140;
const SCROLL_BOTTOM_THRESHOLD = 8;

function resolveActiveSection(elements: HTMLElement[]): string {
  if (elements.length === 0) return "";

  const atBottom =
    window.scrollY + window.innerHeight >=
    document.documentElement.scrollHeight - SCROLL_BOTTOM_THRESHOLD;

  if (atBottom) {
    return elements[elements.length - 1].id;
  }

  let activeId = elements[0].id;

  for (const el of elements) {
    if (el.getBoundingClientRect().top <= ACTIVATION_OFFSET) {
      activeId = el.id;
    }
  }

  return activeId;
}

/**
 * Tracks which section id is currently active while scrolling.
 * Used by FormNav for sidebar / pill highlighting.
 */
export function useActiveSection(sectionIds: string[], enabled = true) {
  const idsKey = sectionIds.join("|");
  const ids = useMemo(() => idsKey.split("|").filter(Boolean), [idsKey]);
  const [activeId, setActiveId] = useState(ids[0] ?? "");

  const resolvedActiveId = ids.includes(activeId) ? activeId : (ids[0] ?? "");

  useEffect(() => {
    if (!enabled || ids.length === 0) return;

    const elements = ids
      .map((id) => document.getElementById(id))
      .filter((el): el is HTMLElement => Boolean(el));

    if (elements.length === 0) return;

    let frame = 0;

    const update = () => {
      setActiveId(resolveActiveSection(elements));
    };

    const scheduleUpdate = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(update);
    };

    update();
    window.addEventListener("scroll", scheduleUpdate, { passive: true });
    window.addEventListener("resize", scheduleUpdate);

    const resizeObserver = new ResizeObserver(scheduleUpdate);
    resizeObserver.observe(document.documentElement);

    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("scroll", scheduleUpdate);
      window.removeEventListener("resize", scheduleUpdate);
      resizeObserver.disconnect();
    };
  }, [enabled, ids]);

  return resolvedActiveId;
}
