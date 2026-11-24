// SkyPath progressive enhancement.
//
// The page is complete without this script: the form is a plain GET and the
// server renders every state. When the script runs it adds, on top of that:
//
//   - browser-side validation with the API's wording, shown as the user
//     leaves a field and on submit;
//   - upper-casing of airport codes and a live hint naming the airport;
//   - a control that swaps origin and destination.
//
// The root element gets the "is-enhanced" class once the script has taken
// over; the stylesheet only reveals script-driven controls under it.
"use strict";

(() => {
  const form = document.getElementById("search-form");
  if (!form || !("noValidate" in form) || typeof Map !== "function") {
    return;
  }

  const fields = {
    origin: document.getElementById("origin"),
    destination: document.getElementById("destination"),
    date: document.getElementById("date"),
  };
  if (!fields.origin || !fields.destination || !fields.date) {
    return;
  }

  const FIELD_LABELS = { origin: "Origin", destination: "Destination", date: "Date" };
  const CODE_PATTERN = /^[A-Za-z]{3}$/;

  // Code -> "City · Name", read from the suggestions the server rendered, so
  // the hint matches the server's wording exactly.
  const airports = new Map();
  for (const option of document.querySelectorAll("#airports option")) {
    airports.set(option.value.trim().toUpperCase(), option.textContent.trim());
  }

  // Fields whose message the user has seen. They are revalidated as they
  // change, so a message disappears as soon as the problem is fixed.
  const touched = new Set();
  // Fields edited since they were last validated.
  const dirty = new Set();

  document.documentElement.classList.add("is-enhanced");
  // Our own messages replace the browser's validation bubbles.
  form.noValidate = true;

  // --- Validation ----------------------------------------------------------

  /** The message for one airport field judged on its own, or "". */
  function codeMessage(name) {
    const value = fields[name].value.trim();
    const label = FIELD_LABELS[name];
    if (value === "") {
      return `${label} is required.`;
    }
    if (!CODE_PATTERN.test(value)) {
      return `${label} must be a three-letter airport code.`;
    }
    const code = value.toUpperCase();
    // Without suggestions (the list failed to load) only the server can tell.
    if (airports.size > 0 && !airports.has(code)) {
      return `Unknown airport code ${code}.`;
    }
    return "";
  }

  /** The message for a field, using the same rules and order as the API. */
  function messageFor(name) {
    if (name === "date") {
      if (fields.date.validity && fields.date.validity.badInput) {
        return "Date must be a valid date in YYYY-MM-DD format.";
      }
      return fields.date.value.trim() === "" ? "Date is required." : "";
    }
    const own = codeMessage(name);
    if (own !== "" || name === "origin") {
      return own;
    }
    // Like the API, only compare once both codes are valid on their own.
    const same =
      codeMessage("origin") === "" &&
      fields.origin.value.trim().toUpperCase() === fields.destination.value.trim().toUpperCase();
    return same ? "Destination must be different from origin." : "";
  }

  function showMessage(name, text) {
    const message = document.getElementById(`${name}-error`);
    if (message) {
      message.textContent = text;
    }
    if (text) {
      fields[name].setAttribute("aria-invalid", "true");
    } else {
      fields[name].removeAttribute("aria-invalid");
    }
  }

  /** Validates one field, shows the result and returns the message. */
  function validate(name) {
    const text = messageFor(name);
    showMessage(name, text);
    touched.add(name);
    dirty.delete(name);
    return text;
  }

  /** Validates every field; returns the names of the invalid ones in order. */
  function validateAll() {
    return Object.keys(fields).filter((name) => validate(name) !== "");
  }

  // Fields the server already marked invalid count as seen.
  for (const name of Object.keys(fields)) {
    if (fields[name].getAttribute("aria-invalid") === "true") {
      touched.add(name);
    }
  }

  // --- Airport inputs ------------------------------------------------------

  function updateHint(name) {
    const hint = document.getElementById(`${name}-hint`);
    if (hint) {
      hint.textContent = airports.get(fields[name].value.trim().toUpperCase()) || "";
    }
  }

  /** Upper-cases the value in place without moving the caret. */
  function upperCase(input) {
    const upper = input.value.toUpperCase();
    if (upper === input.value) {
      return;
    }
    const { selectionStart, selectionEnd } = input;
    input.value = upper;
    try {
      input.setSelectionRange(selectionStart, selectionEnd);
    } catch {
      // Some input types do not support selection; the value is still right.
    }
  }

  /** Revalidates fields whose messages are showing after a value changed. */
  function revalidateTouched(names) {
    for (const name of names) {
      if (touched.has(name)) {
        validate(name);
      }
    }
  }

  for (const name of ["origin", "destination"]) {
    const input = fields[name];
    input.addEventListener("input", (event) => {
      // Leave the value alone while an input method is composing text.
      if (!event.isComposing) {
        upperCase(input);
      }
      dirty.add(name);
      updateHint(name);
      // The destination rule depends on the origin, so an origin change
      // rechecks both.
      revalidateTouched(name === "origin" ? ["origin", "destination"] : ["destination"]);
    });
  }

  fields.date.addEventListener("input", () => {
    dirty.add("date");
    revalidateTouched(["date"]);
  });

  // Validate as the user leaves a field, but only once they have edited it or
  // when it already shows a message: tabbing through an empty form is not an
  // error.
  for (const name of Object.keys(fields)) {
    fields[name].addEventListener("blur", () => {
      if (dirty.has(name) || touched.has(name)) {
        validate(name);
      }
    });
  }

  // --- Swap ----------------------------------------------------------------

  const swapButton = document.getElementById("swap");
  const swapArea = swapButton && swapButton.closest(".search-form__swap");
  if (swapButton && swapArea) {
    swapArea.hidden = false;
    swapButton.addEventListener("click", () => {
      const { origin, destination } = fields;
      [origin.value, destination.value] = [destination.value, origin.value];
      updateHint("origin");
      updateHint("destination");
      if (touched.has("origin") || touched.has("destination")) {
        validate("origin");
        validate("destination");
      }
      // Turn the icon half a revolution per swap, as feedback.
      swapButton.classList.toggle("is-swapped");
    });
  }

  // --- Submit --------------------------------------------------------------

  form.addEventListener("submit", (event) => {
    const invalid = validateAll();
    if (invalid.length > 0) {
      event.preventDefault();
      // The field's message is linked with aria-describedby, so moving focus
      // there is enough for it to be read out.
      fields[invalid[0]].focus();
    }
  });
})();
