import { forPerson, forTitle } from "./sortname";
import { describe, expect, it } from "vitest";

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
    expect(forPerson("Martin Luther King Jr.")).toBe(
      "King, Martin Luther, Jr.",
    );
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
