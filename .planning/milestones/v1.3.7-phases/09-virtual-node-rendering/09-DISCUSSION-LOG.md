# Phase 9: Virtual Node Rendering - Discussion Log

**Date:** 2026-03-31
**Mode:** Interactive (discuss)

## Areas Selected

All 4 gray areas selected: Card visual identity, Card header layout, Link metric labels, Area color and glow

## Discussion

### Card Visual Identity

**Q1: How should virtual node cards be visually distinguished from physical device cards?**
- Options: Dashed border + muted background | Solid border, distinct background tint | Same style as physical | You decide
- **Selected:** Dashed border + muted background
- Notes: Like ghost nodes but interactive. Dashed outline signals 'not a real device'.

**Q2: Should the dashed border style be distinct from ghost nodes?**
- Options: Same dashed style, different opacity | Different dash pattern | You decide
- **Selected:** Same dashed style, different opacity
- Notes: Virtual nodes fully opaque and interactive; ghost nodes semi-transparent and non-interactive. Opacity is enough to distinguish.

### Card Header Layout

**Q3: How should the virtual node card header be arranged?**
- Options: Centered icon + name below | Horizontal like physical cards | You decide
- **Selected:** Centered icon + name below
- Notes: Vertical layout with icon centered on top, display_name below. StatusDot next to name for IP-bearing cards.

**Q4: What size should the subtype Material Symbol icon be?**
- Options: Large (28-32px) | Medium (22-24px) | You decide
- **Selected:** Medium (22-24px)

**Q5: Font styling for display name text?**
- Options: font-mono like hostnames | font-sans (Outfit) | You decide
- **Selected:** font-mono like hostnames
- Notes: Consistent with physical cards. Truncated with ellipsis.

### Link Metric Labels

**Q6: How should throughput labels display for physical→virtual links?**
- Options: Rates only, no interface name | Show interface name from physical side | You decide
- **Selected:** Rates only, no interface name
- Notes: Show '↑ 1.2Mbps ↓ 3.4Mbps' without naming the interface.

**Q7: Bandwidth label on virtual links — show single speed or hide?**
- Options: Show single speed | Hide bandwidth label | You decide
- **Selected:** Show single speed
- Notes: Display real interface's negotiated speed (e.g., '1Gbps'). No mismatch indicator.

### Area Color and Glow

**Q8: Should virtual nodes receive area color treatment?**
- Options: Yes, same area colors | No area colors, neutral only | Muted area colors
- **Selected:** Yes, same area colors
- Notes: Virtual nodes in an area get same gradient background as physical devices.

**Q9: Should virtual nodes show status-based glow effects?**
- Options: Yes, same glow behavior | No glow for virtual nodes | Glow only for IP-bearing
- **Selected:** Yes, same glow behavior
- Notes: Both IP-bearing and no-IP virtual nodes get glow treatment consistent with physical devices.

---

*Discussion completed: 2026-03-31*
*Output: 09-CONTEXT.md*
