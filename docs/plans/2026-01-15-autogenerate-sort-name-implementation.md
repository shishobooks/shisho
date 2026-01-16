# Autogenerate Sort Name Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve sort name editing UX with checkbox + live preview, replacing unclear placeholder text.

**Architecture:** Port Go sort name algorithms to TypeScript utilities, create reusable `SortNameInput` component with checkbox/input combo, integrate into edit dialogs.

**Tech Stack:** React, TypeScript, Vitest, Radix UI Checkbox

---

## Task 1: Sort Name Utility - forTitle function

**Files:**
- Create: `app/utils/sortname.ts`
- Create: `app/utils/sortname.test.ts`

**Step 1: Write the failing test for forTitle**

```typescript
// app/utils/sortname.test.ts
import { describe, expect, it } from "vitest";

import { forTitle } from "./sortname";

describe("forTitle", () => {
  it("moves 'The' to end", () => {
    expect(forTitle("The Hobbit")).toBe("Hobbit, The");
  });

  it("moves 'A' to end", () => {
    expect(forTitle("A Tale of Two Cities")).toBe("Tale of Two Cities, A");
  });

  it("moves 'An' to end", () => {
    expect(forTitle("An American Tragedy")).toBe("American Tragedy, An");
  });

  it("handles lowercase articles", () => {
    expect(forTitle("the hobbit")).toBe("hobbit, the");
  });

  it("handles uppercase articles", () => {
    expect(forTitle("THE HOBBIT")).toBe("HOBBIT, THE");
  });

  it("returns unchanged when no leading article", () => {
    expect(forTitle("Lord of the Rings")).toBe("Lord of the Rings");
  });

  it("returns unchanged when article in middle only", () => {
    expect(forTitle("Return of the King")).toBe("Return of the King");
  });

  it("handles empty string", () => {
    expect(forTitle("")).toBe("");
  });

  it("handles whitespace only", () => {
    expect(forTitle("   ")).toBe("");
  });

  it("returns single word unchanged", () => {
    expect(forTitle("Dune")).toBe("Dune");
  });

  it("returns 'The' alone unchanged", () => {
    expect(forTitle("The")).toBe("The");
  });

  it("handles real-world example: The Lord of the Rings", () => {
    expect(forTitle("The Lord of the Rings")).toBe("Lord of the Rings, The");
  });

  it("handles real-world example: A Game of Thrones", () => {
    expect(forTitle("A Game of Thrones")).toBe("Game of Thrones, A");
  });
});
```

**Step 2: Run test to verify it fails**

Run: `yarn vitest run app/utils/sortname.test.ts`
Expected: FAIL with "Cannot find module './sortname'"

**Step 3: Write forTitle implementation**

```typescript
// app/utils/sortname.ts

/**
 * Articles to strip from the beginning of titles.
 * Moved to end (e.g., "The Hobbit" → "Hobbit, The").
 */
const TITLE_ARTICLES = ["The", "A", "An"];

/**
 * Generates a sort title from a display title.
 * Leading articles are moved to the end.
 *
 * @example forTitle("The Hobbit") // "Hobbit, The"
 * @example forTitle("A Tale of Two Cities") // "Tale of Two Cities, A"
 */
export function forTitle(title: string): string {
  title = title.trim();
  if (!title) {
    return "";
  }

  for (const article of TITLE_ARTICLES) {
    const prefix = article + " ";
    if (
      title.length > prefix.length &&
      title.slice(0, prefix.length).toLowerCase() === prefix.toLowerCase()
    ) {
      // Extract actual article (preserving original case)
      const actualArticle = title.slice(0, article.length);
      const rest = title.slice(prefix.length).trim();
      if (rest) {
        return rest + ", " + actualArticle;
      }
    }
  }

  return title;
}
```

**Step 4: Run test to verify it passes**

Run: `yarn vitest run app/utils/sortname.test.ts`
Expected: PASS

**Step 5: Commit**

```bash
git add app/utils/sortname.ts app/utils/sortname.test.ts
git commit -m "$(cat <<'EOF'
[Feature] Add forTitle sort name utility

Port of Go forTitle algorithm that moves leading articles
(The, A, An) to the end for proper alphabetical sorting.
EOF
)"
```

---

## Task 2: Sort Name Utility - forPerson function

**Files:**
- Modify: `app/utils/sortname.ts`
- Modify: `app/utils/sortname.test.ts`

**Step 1: Add failing tests for forPerson**

Add to `app/utils/sortname.test.ts`:

```typescript
import { forPerson, forTitle } from "./sortname";

describe("forPerson", () => {
  // Basic name inversion
  it("inverts simple two-part name", () => {
    expect(forPerson("Stephen King")).toBe("King, Stephen");
  });

  it("inverts three-part name", () => {
    expect(forPerson("Martin Luther King")).toBe("King, Martin Luther");
  });

  // Generational suffixes (preserved)
  it("preserves Jr. suffix", () => {
    expect(forPerson("Robert Downey Jr.")).toBe("Downey, Robert, Jr.");
  });

  it("preserves Jr without period", () => {
    expect(forPerson("Robert Downey Jr")).toBe("Downey, Robert, Jr");
  });

  it("preserves Sr. suffix", () => {
    expect(forPerson("John Smith Sr.")).toBe("Smith, John, Sr.");
  });

  it("preserves III suffix", () => {
    expect(forPerson("John Smith III")).toBe("Smith, John, III");
  });

  it("handles Martin Luther King Jr.", () => {
    expect(forPerson("Martin Luther King Jr.")).toBe("King, Martin Luther, Jr.");
  });

  // Academic suffixes (stripped)
  it("strips PhD suffix", () => {
    expect(forPerson("Jane Doe PhD")).toBe("Doe, Jane");
  });

  it("strips Ph.D. suffix", () => {
    expect(forPerson("Jane Doe Ph.D.")).toBe("Doe, Jane");
  });

  it("strips MD suffix", () => {
    expect(forPerson("John Smith MD")).toBe("Smith, John");
  });

  it("strips multiple academic suffixes", () => {
    expect(forPerson("John Doe MD PhD")).toBe("Doe, John");
  });

  // Mixed generational and academic
  it("preserves Jr. but strips PhD", () => {
    expect(forPerson("John Smith Jr. PhD")).toBe("Smith, John, Jr.");
  });

  // Prefixes (stripped)
  it("strips Dr. prefix", () => {
    expect(forPerson("Dr. Sarah Connor")).toBe("Connor, Sarah");
  });

  it("strips Dr without period", () => {
    expect(forPerson("Dr Sarah Connor")).toBe("Connor, Sarah");
  });

  it("strips Mr. prefix", () => {
    expect(forPerson("Mr. John Smith")).toBe("Smith, John");
  });

  it("strips Mrs. prefix", () => {
    expect(forPerson("Mrs. Jane Doe")).toBe("Doe, Jane");
  });

  it("strips Prof. prefix", () => {
    expect(forPerson("Prof. Albert Einstein")).toBe("Einstein, Albert");
  });

  it("strips Sir prefix", () => {
    expect(forPerson("Sir Isaac Newton")).toBe("Newton, Isaac");
  });

  // Prefix and suffix combined
  it("strips Dr. and PhD", () => {
    expect(forPerson("Dr. John Smith PhD")).toBe("Smith, John");
  });

  // Particles (moved to end)
  it("handles van Beethoven", () => {
    expect(forPerson("Ludwig van Beethoven")).toBe("Beethoven, Ludwig van");
  });

  it("handles von Neumann", () => {
    expect(forPerson("John von Neumann")).toBe("Neumann, John von");
  });

  it("handles da Vinci", () => {
    expect(forPerson("Leonardo da Vinci")).toBe("Vinci, Leonardo da");
  });

  it("handles de Gaulle", () => {
    expect(forPerson("Charles de Gaulle")).toBe("Gaulle, Charles de");
  });

  it("handles del Toro", () => {
    expect(forPerson("Guillermo del Toro")).toBe("Toro, Guillermo del");
  });

  it("handles van der Waals", () => {
    expect(forPerson("Johannes van der Waals")).toBe("Waals, Johannes van der");
  });

  // Particle with suffix
  it("handles particle with Jr.", () => {
    expect(forPerson("John van Smith Jr.")).toBe("Smith, John van, Jr.");
  });

  // Edge cases
  it("handles empty string", () => {
    expect(forPerson("")).toBe("");
  });

  it("handles whitespace only", () => {
    expect(forPerson("   ")).toBe("");
  });

  it("returns single name unchanged", () => {
    expect(forPerson("Madonna")).toBe("Madonna");
  });

  it("handles single name with whitespace", () => {
    expect(forPerson("  Cher  ")).toBe("Cher");
  });

  // Real-world examples
  it("handles J.R.R. Tolkien", () => {
    expect(forPerson("J.R.R. Tolkien")).toBe("Tolkien, J.R.R.");
  });

  it("handles George R.R. Martin", () => {
    expect(forPerson("George R.R. Martin")).toBe("Martin, George R.R.");
  });

  it("handles H.P. Lovecraft", () => {
    expect(forPerson("H.P. Lovecraft")).toBe("Lovecraft, H.P.");
  });
});
```

**Step 2: Run tests to verify forPerson fails**

Run: `yarn vitest run app/utils/sortname.test.ts`
Expected: FAIL with "forPerson is not exported"

**Step 3: Implement forPerson**

Add to `app/utils/sortname.ts`:

```typescript
/**
 * Generational suffixes - preserved in sort name as they distinguish people.
 */
const GENERATIONAL_SUFFIXES = [
  "Jr.",
  "Jr",
  "Sr.",
  "Sr",
  "Junior",
  "Senior",
  "I",
  "II",
  "III",
  "IV",
  "V",
];

/**
 * Academic suffixes - stripped from sort name (credentials, not name).
 */
const ACADEMIC_SUFFIXES = [
  "PhD",
  "Ph.D",
  "Ph.D.",
  "PsyD",
  "Psy.D",
  "Psy.D.",
  "MD",
  "M.D",
  "M.D.",
  "DO",
  "D.O",
  "D.O.",
  "DDS",
  "D.D.S",
  "D.D.S.",
  "JD",
  "J.D",
  "J.D.",
  "EdD",
  "Ed.D",
  "Ed.D.",
  "LLD",
  "LL.D",
  "LL.D.",
  "MBA",
  "M.B.A",
  "M.B.A.",
  "MS",
  "M.S",
  "M.S.",
  "MA",
  "M.A",
  "M.A.",
  "BA",
  "B.A",
  "B.A.",
  "BS",
  "B.S",
  "B.S.",
  "RN",
  "R.N",
  "R.N.",
  "Esq",
  "Esq.",
];

/**
 * Prefixes - honorifics/titles stripped from sort name.
 */
const PREFIXES = [
  "Dr.",
  "Dr",
  "Mr.",
  "Mr",
  "Mrs.",
  "Mrs",
  "Ms.",
  "Ms",
  "Prof.",
  "Prof",
  "Rev.",
  "Rev",
  "Fr.",
  "Fr",
  "Sir",
  "Dame",
  "Lord",
  "Lady",
];

/**
 * Name particles - moved to end with given name (library style).
 * Example: "Ludwig van Beethoven" → "Beethoven, Ludwig van"
 */
const PARTICLES = [
  "van",
  "von",
  "de",
  "da",
  "di",
  "du",
  "del",
  "della",
  "la",
  "le",
  "el",
  "al",
  "bin",
  "ibn",
];

function isPrefix(word: string): boolean {
  return PREFIXES.some((p) => p.toLowerCase() === word.toLowerCase());
}

function isGenerationalSuffix(word: string): boolean {
  // Remove trailing comma if present
  const cleaned = word.replace(/,$/, "");
  return GENERATIONAL_SUFFIXES.some(
    (s) => s.toLowerCase() === cleaned.toLowerCase(),
  );
}

function isAcademicSuffix(word: string): boolean {
  // Remove trailing comma if present
  const cleaned = word.replace(/,$/, "");
  return ACADEMIC_SUFFIXES.some(
    (s) => s.toLowerCase() === cleaned.toLowerCase(),
  );
}

function isParticle(word: string): boolean {
  return PARTICLES.some((p) => p.toLowerCase() === word.toLowerCase());
}

/**
 * Generates a sort name from a person's display name.
 * Converts to "Last, First Middle" format with proper handling of:
 * - Prefixes (Dr., Mr., etc.) - stripped
 * - Academic suffixes (PhD, MD, etc.) - stripped
 * - Generational suffixes (Jr., III, etc.) - preserved
 * - Particles (van, von, de, etc.) - moved to end with given name
 *
 * @example forPerson("Stephen King") // "King, Stephen"
 * @example forPerson("Martin Luther King Jr.") // "King, Martin Luther, Jr."
 * @example forPerson("Ludwig van Beethoven") // "Beethoven, Ludwig van"
 */
export function forPerson(name: string): string {
  name = name.trim();
  if (!name) {
    return "";
  }

  let parts = name.split(/\s+/);
  if (parts.length === 0) {
    return "";
  }

  // Single word name - return as is
  if (parts.length === 1) {
    return name;
  }

  // Strip prefixes from the beginning
  while (parts.length > 1 && isPrefix(parts[0])) {
    parts = parts.slice(1);
  }

  if (parts.length === 0) {
    return name; // All parts were prefixes, return original
  }

  if (parts.length === 1) {
    return parts[0];
  }

  // Extract and strip academic suffixes from the end, preserve generational suffixes
  const generationalSuffixes: string[] = [];
  while (parts.length > 1) {
    const last = parts[parts.length - 1];
    if (isGenerationalSuffix(last)) {
      generationalSuffixes.unshift(last);
      parts = parts.slice(0, -1);
    } else if (isAcademicSuffix(last)) {
      parts = parts.slice(0, -1);
    } else {
      break;
    }
  }

  if (parts.length === 0) {
    return name; // All parts were suffixes, return original
  }

  if (parts.length === 1) {
    // Only one part left after stripping
    if (generationalSuffixes.length > 0) {
      return parts[0] + ", " + generationalSuffixes.join(", ");
    }
    return parts[0];
  }

  // Find the last word (surname)
  const surname = parts[parts.length - 1];
  let givenParts = parts.slice(0, -1);

  // Collect consecutive particles at the end of givenParts
  const particleParts: string[] = [];
  while (givenParts.length > 0) {
    const last = givenParts[givenParts.length - 1];
    if (isParticle(last)) {
      particleParts.unshift(last);
      givenParts = givenParts.slice(0, -1);
    } else {
      break;
    }
  }

  // Build the sort name
  let result = surname;

  // Add given name parts
  if (givenParts.length > 0 || particleParts.length > 0) {
    result += ", ";
    if (givenParts.length > 0) {
      result += givenParts.join(" ");
    }
    if (particleParts.length > 0) {
      if (givenParts.length > 0) {
        result += " ";
      }
      result += particleParts.join(" ");
    }
  }

  // Add generational suffixes
  if (generationalSuffixes.length > 0) {
    result += ", " + generationalSuffixes.join(", ");
  }

  return result;
}
```

**Step 4: Run tests to verify all pass**

Run: `yarn vitest run app/utils/sortname.test.ts`
Expected: PASS (all forTitle and forPerson tests)

**Step 5: Commit**

```bash
git add app/utils/sortname.ts app/utils/sortname.test.ts
git commit -m "$(cat <<'EOF'
[Feature] Add forPerson sort name utility

Port of Go forPerson algorithm that converts names to
"Last, First" format with proper handling of prefixes,
academic suffixes, generational suffixes, and particles.
EOF
)"
```

---

## Task 3: SortNameInput Component

**Files:**
- Create: `app/components/common/SortNameInput.tsx`
- Create: `app/components/common/SortNameInput.test.tsx`

**Step 1: Write the failing test**

```typescript
// app/components/common/SortNameInput.test.tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { SortNameInput } from "./SortNameInput";

describe("SortNameInput", () => {
  it("shows checkbox and input", () => {
    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={() => {}}
        sortValue=""
        source="manual"
        type="person"
      />,
    );

    expect(screen.getByRole("checkbox")).toBeInTheDocument();
    expect(screen.getByRole("textbox")).toBeInTheDocument();
  });

  it("checkbox is checked when source is not manual", () => {
    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={() => {}}
        sortValue="King, Stephen"
        source="filepath"
        type="person"
      />,
    );

    expect(screen.getByRole("checkbox")).toBeChecked();
  });

  it("checkbox is unchecked when source is manual", () => {
    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={() => {}}
        sortValue="King, S."
        source="manual"
        type="person"
      />,
    );

    expect(screen.getByRole("checkbox")).not.toBeChecked();
  });

  it("shows live preview when checkbox is checked", () => {
    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={() => {}}
        sortValue=""
        source="filepath"
        type="person"
      />,
    );

    expect(screen.getByRole("textbox")).toHaveValue("King, Stephen");
    expect(screen.getByRole("textbox")).toBeDisabled();
  });

  it("updates preview when nameValue changes", () => {
    const { rerender } = render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={() => {}}
        sortValue=""
        source="filepath"
        type="person"
      />,
    );

    expect(screen.getByRole("textbox")).toHaveValue("King, Stephen");

    rerender(
      <SortNameInput
        nameValue="J.R.R. Tolkien"
        onChange={() => {}}
        sortValue=""
        source="filepath"
        type="person"
      />,
    );

    expect(screen.getByRole("textbox")).toHaveValue("Tolkien, J.R.R.");
  });

  it("calls onChange with empty string when checkbox is checked", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={onChange}
        sortValue="King, S."
        source="manual"
        type="person"
      />,
    );

    await user.click(screen.getByRole("checkbox"));

    expect(onChange).toHaveBeenCalledWith("");
  });

  it("enables input and pre-fills with generated value when unchecked", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={onChange}
        sortValue=""
        source="filepath"
        type="person"
      />,
    );

    // Initially disabled
    expect(screen.getByRole("textbox")).toBeDisabled();

    // Uncheck the checkbox
    await user.click(screen.getByRole("checkbox"));

    // Now enabled with generated value
    expect(screen.getByRole("textbox")).not.toBeDisabled();
    expect(screen.getByRole("textbox")).toHaveValue("King, Stephen");
    expect(onChange).toHaveBeenCalledWith("King, Stephen");
  });

  it("uses forTitle for title type", () => {
    render(
      <SortNameInput
        nameValue="The Hobbit"
        onChange={() => {}}
        sortValue=""
        source="filepath"
        type="title"
      />,
    );

    expect(screen.getByRole("textbox")).toHaveValue("Hobbit, The");
  });

  it("uses forPerson for person type", () => {
    render(
      <SortNameInput
        nameValue="Ludwig van Beethoven"
        onChange={() => {}}
        sortValue=""
        source="filepath"
        type="person"
      />,
    );

    expect(screen.getByRole("textbox")).toHaveValue("Beethoven, Ludwig van");
  });

  it("shows correct label for title type", () => {
    render(
      <SortNameInput
        nameValue="The Hobbit"
        onChange={() => {}}
        sortValue=""
        source="filepath"
        type="title"
      />,
    );

    expect(screen.getByText("Autogenerate sort title")).toBeInTheDocument();
  });

  it("shows correct label for person type", () => {
    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={() => {}}
        sortValue=""
        source="filepath"
        type="person"
      />,
    );

    expect(screen.getByText("Autogenerate sort name")).toBeInTheDocument();
  });

  it("allows typing when checkbox is unchecked", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(
      <SortNameInput
        nameValue="Stephen King"
        onChange={onChange}
        sortValue="Custom Sort"
        source="manual"
        type="person"
      />,
    );

    const input = screen.getByRole("textbox");
    await user.clear(input);
    await user.type(input, "New Value");

    expect(onChange).toHaveBeenLastCalledWith("New Value");
  });
});
```

**Step 2: Run test to verify it fails**

Run: `yarn vitest run app/components/common/SortNameInput.test.tsx`
Expected: FAIL with "Cannot find module './SortNameInput'"

**Step 3: Write the SortNameInput component**

```typescript
// app/components/common/SortNameInput.tsx
import { useEffect, useMemo, useState } from "react";

import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { type DataSource, DataSourceManual } from "@/types";
import { forPerson, forTitle } from "@/utils/sortname";

interface SortNameInputProps {
  /** The name/title being edited (for live preview) */
  nameValue: string;
  /** Current sort name/title value */
  sortValue: string;
  /** Source of the current value ("manual" or other) */
  source: DataSource;
  /** Which algorithm to use */
  type: "title" | "person";
  /** Called with empty string (auto) or actual value (manual) */
  onChange: (value: string) => void;
}

export function SortNameInput({
  nameValue,
  sortValue,
  source,
  type,
  onChange,
}: SortNameInputProps) {
  // Checkbox starts checked if source is not manual
  const [isAuto, setIsAuto] = useState(source !== DataSourceManual);
  // Track the manual value separately
  const [manualValue, setManualValue] = useState(sortValue);

  // Compute the auto-generated value
  const generatedValue = useMemo(() => {
    return type === "title" ? forTitle(nameValue) : forPerson(nameValue);
  }, [nameValue, type]);

  // The displayed value depends on mode
  const displayValue = isAuto ? generatedValue : manualValue;

  // Update manual value when sortValue prop changes (dialog reopened)
  useEffect(() => {
    setManualValue(sortValue);
    setIsAuto(source !== DataSourceManual);
  }, [sortValue, source]);

  const handleCheckboxChange = (checked: boolean) => {
    setIsAuto(checked);
    if (checked) {
      // Switching to auto mode - send empty string
      onChange("");
    } else {
      // Switching to manual mode - pre-fill with generated value
      setManualValue(generatedValue);
      onChange(generatedValue);
    }
  };

  const handleInputChange = (value: string) => {
    setManualValue(value);
    onChange(value);
  };

  const label = type === "title" ? "Autogenerate sort title" : "Autogenerate sort name";

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <Checkbox
          checked={isAuto}
          id="autogenerate-sort"
          onCheckedChange={handleCheckboxChange}
        />
        <Label className="font-normal" htmlFor="autogenerate-sort">
          {label}
        </Label>
      </div>
      <Input
        disabled={isAuto}
        onChange={(e) => handleInputChange(e.target.value)}
        value={displayValue}
      />
    </div>
  );
}
```

**Step 4: Run tests to verify they pass**

Run: `yarn vitest run app/components/common/SortNameInput.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add app/components/common/SortNameInput.tsx app/components/common/SortNameInput.test.tsx
git commit -m "$(cat <<'EOF'
[Feature] Add SortNameInput component

Reusable checkbox + input combo that shows live preview
of auto-generated sort names when checked, and allows
manual editing when unchecked.
EOF
)"
```

---

## Task 4: Integrate SortNameInput into MetadataEditDialog

**Files:**
- Modify: `app/components/library/MetadataEditDialog.tsx:23-31` (props interface)
- Modify: `app/components/library/MetadataEditDialog.tsx:91-104` (sort name field)

**Step 1: Add sortNameSource prop to interface**

In `app/components/library/MetadataEditDialog.tsx`, update the interface:

```typescript
// Change from:
interface MetadataEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: EntityType;
  entityName: string;
  sortName?: string;
  onSave: (data: { name: string; sort_name?: string }) => Promise<void>;
  isPending: boolean;
}

// Change to:
interface MetadataEditDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  entityType: EntityType;
  entityName: string;
  sortName?: string;
  sortNameSource?: DataSource;
  onSave: (data: { name: string; sort_name?: string }) => Promise<void>;
  isPending: boolean;
}
```

**Step 2: Add import for SortNameInput and DataSource**

At the top of `app/components/library/MetadataEditDialog.tsx`:

```typescript
import { SortNameInput } from "@/components/common/SortNameInput";
import { type DataSource, DataSourceManual } from "@/types";
```

**Step 3: Update component to use sortNameSource prop**

```typescript
export function MetadataEditDialog({
  open,
  onOpenChange,
  entityType,
  entityName,
  sortName,
  sortNameSource,
  onSave,
  isPending,
}: MetadataEditDialogProps) {
```

**Step 4: Replace sort_name input with SortNameInput**

Replace the sort_name field block (lines ~91-104):

```typescript
// Change from:
{hasSortName && (
  <div className="space-y-2">
    <Label htmlFor="sort_name">Sort Name</Label>
    <Input
      id="sort_name"
      onChange={(e) => setEditSortName(e.target.value)}
      placeholder="Leave empty to auto-generate"
      value={editSortName}
    />
    <p className="text-xs text-muted-foreground">
      Used for sorting. Clear to regenerate automatically.
    </p>
  </div>
)}

// Change to:
{hasSortName && (
  <div className="space-y-2">
    <Label>Sort Name</Label>
    <SortNameInput
      nameValue={name}
      onChange={setEditSortName}
      sortValue={sortName || ""}
      source={sortNameSource || DataSourceManual}
      type={entityType === "person" ? "person" : "title"}
    />
  </div>
)}
```

**Step 5: Run lint and tests**

Run: `yarn lint && yarn vitest run`
Expected: PASS

**Step 6: Commit**

```bash
git add app/components/library/MetadataEditDialog.tsx
git commit -m "$(cat <<'EOF'
[Feature] Use SortNameInput in MetadataEditDialog

Replace placeholder-based sort name input with checkbox +
live preview component for clearer UX.
EOF
)"
```

---

## Task 5: Pass sortNameSource from PersonDetail

**Files:**
- Modify: `app/components/pages/PersonDetail.tsx:235-243` (MetadataEditDialog props)

**Step 1: Add sortNameSource prop to MetadataEditDialog**

In `app/components/pages/PersonDetail.tsx`, find the MetadataEditDialog usage and add the prop:

```typescript
// Change from:
<MetadataEditDialog
  entityName={person.name}
  entityType="person"
  isPending={updatePersonMutation.isPending}
  onOpenChange={setEditOpen}
  onSave={handleEdit}
  open={editOpen}
  sortName={person.sort_name}
/>

// Change to:
<MetadataEditDialog
  entityName={person.name}
  entityType="person"
  isPending={updatePersonMutation.isPending}
  onOpenChange={setEditOpen}
  onSave={handleEdit}
  open={editOpen}
  sortName={person.sort_name}
  sortNameSource={person.sort_name_source}
/>
```

**Step 2: Run lint**

Run: `yarn lint`
Expected: PASS

**Step 3: Commit**

```bash
git add app/components/pages/PersonDetail.tsx
git commit -m "$(cat <<'EOF'
[Feature] Pass sort_name_source to MetadataEditDialog in PersonDetail

Enables checkbox to show correct initial state based on
whether sort name was auto-generated or manually set.
EOF
)"
```

---

## Task 6: Pass sortNameSource from SeriesDetail

**Files:**
- Modify: `app/components/pages/SeriesDetail.tsx:189-197` (MetadataEditDialog props)

**Step 1: Add sortNameSource prop to MetadataEditDialog**

In `app/components/pages/SeriesDetail.tsx`, find the MetadataEditDialog usage and add the prop:

```typescript
// Change from:
<MetadataEditDialog
  entityName={series.name}
  entityType="series"
  isPending={updateSeriesMutation.isPending}
  onOpenChange={setEditOpen}
  onSave={handleEdit}
  open={editOpen}
  sortName={series.sort_name}
/>

// Change to:
<MetadataEditDialog
  entityName={series.name}
  entityType="series"
  isPending={updateSeriesMutation.isPending}
  onOpenChange={setEditOpen}
  onSave={handleEdit}
  open={editOpen}
  sortName={series.sort_name}
  sortNameSource={series.sort_name_source}
/>
```

**Step 2: Run lint**

Run: `yarn lint`
Expected: PASS

**Step 3: Commit**

```bash
git add app/components/pages/SeriesDetail.tsx
git commit -m "$(cat <<'EOF'
[Feature] Pass sort_name_source to MetadataEditDialog in SeriesDetail

Enables checkbox to show correct initial state based on
whether sort name was auto-generated or manually set.
EOF
)"
```

---

## Task 7: Integrate SortNameInput into BookEditDialog

**Files:**
- Modify: `app/components/library/BookEditDialog.tsx:1-55` (imports)
- Modify: `app/components/library/BookEditDialog.tsx:392-404` (sort title field)

**Step 1: Add import for SortNameInput**

At the top of `app/components/library/BookEditDialog.tsx`, add:

```typescript
import { SortNameInput } from "@/components/common/SortNameInput";
```

**Step 2: Replace sort_title input with SortNameInput**

Replace the Sort Title field block (lines ~392-404):

```typescript
// Change from:
{/* Sort Title */}
<div className="space-y-2">
  <Label htmlFor="sort_title">Sort Title</Label>
  <Input
    id="sort_title"
    onChange={(e) => setSortTitle(e.target.value)}
    placeholder="Leave empty to auto-generate from title"
    value={sortTitle}
  />
  <p className="text-xs text-muted-foreground">
    Used for sorting. Clear to regenerate automatically.
  </p>
</div>

// Change to:
{/* Sort Title */}
<div className="space-y-2">
  <Label>Sort Title</Label>
  <SortNameInput
    nameValue={title}
    onChange={setSortTitle}
    sortValue={book.sort_title}
    source={book.sort_title_source}
    type="title"
  />
</div>
```

**Step 3: Run lint and tests**

Run: `yarn lint && yarn vitest run`
Expected: PASS

**Step 4: Commit**

```bash
git add app/components/library/BookEditDialog.tsx
git commit -m "$(cat <<'EOF'
[Feature] Use SortNameInput in BookEditDialog

Replace placeholder-based sort title input with checkbox +
live preview component for clearer UX.
EOF
)"
```

---

## Task 8: Run Full Validation

**Step 1: Run make check**

Run: `make check`
Expected: PASS (all tests, lint, type checks)

**Step 2: Manual testing checklist**

Test the following scenarios manually:
1. Edit a person with auto-generated sort name → checkbox should be checked, input disabled
2. Uncheck the checkbox → input enables with generated value
3. Edit the value and save → next time checkbox should be unchecked
4. Re-check the checkbox and save → sort name regenerates
5. Same tests for series
6. Same tests for book sort title

**Step 3: Final commit (if any cleanup needed)**

```bash
git status
# If clean, no action needed
# If changes exist, commit them
```

---

## Task 9: Squash Merge to Master

**Files:** None (git operation)

**Step 1: Ensure all changes are committed**

Run: `git status`
Expected: working tree clean

**Step 2: Switch to master and squash merge**

Run the squash-merge-worktree skill to complete the merge.
