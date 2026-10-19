import assert from "node:assert/strict";
import { test } from "node:test";
import { formatDateInput, formatPlanDate, isValidDateInput } from "../static/formatters.js";

for (const date of ["1970-01-02", "1971-01-01", "1973-03-03", "1973-03-04", "2000-02-29", "2026-11-09", "9999-12-31"]) {
  test(`plan date round-trips in milliseconds: ${date}`, () => {
    const milliseconds = new Date(`${date}T00:00:00`).getTime();
    assert.equal(isValidDateInput(date), true);
    assert.equal(formatDateInput(milliseconds), date);
    assert.equal(formatPlanDate(milliseconds), date.split("-").map(Number).join("/"));
  });
}

test("invalid calendar dates are rejected", () => {
  for (const date of ["", "2026-02-29", "2026-02-30", "2026-13-01", "2026-00-01", "2026-11-00", "2026-1-1"]) {
    assert.equal(isValidDateInput(date), false, date);
  }
});
