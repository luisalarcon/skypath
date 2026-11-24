// SkyPath progressive enhancement.
//
// The page is complete without this script: the form is a plain GET and the
// server renders every state. When the script runs it adds, on top of that:
//
//   - browser-side validation with the API's wording, shown as the user
//     leaves a field and on submit;
//   - upper-casing of airport codes and a live hint naming the airport;
//   - a control that swaps origin and destination;
//   - searches without a full page load: the script fetches the same URL the
//     form would have navigated to, takes the notice area, the results area
//     and the field messages from the returned page and swaps them in, and
//     keeps the address bar and history in step.
//
// Because every response is the ordinary server-rendered page there is one
// rendering path. Whenever something unexpected happens the script falls
// back to a normal navigation and lets the server render the outcome.
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

  // --- In-place search ----------------------------------------------------

  // Everything below needs these; without them the form simply navigates.
  const canFetch =
    typeof window.fetch === "function" &&
    typeof window.AbortController === "function" &&
    typeof window.DOMParser === "function" &&
    typeof window.URLSearchParams === "function" &&
    typeof window.FormData === "function" &&
    !!(window.history && typeof history.pushState === "function");

  // Elements swapped wholesale from a fetched page. Each exists exactly once
  // in every state of the search page.
  const REGION_IDS = ["notice", "results", "origin-error", "destination-error", "date-error"];

  const submitLabel = form.querySelector("[data-loading-label]");
  const idleLabel = submitLabel ? submitLabel.textContent : "";
  const announcer = document.getElementById("announcer");
  const skeleton = document.getElementById("results-skeleton");

  // The search being fetched, so a newer one can abort it.
  let inFlight = null;
  // Path and query of the page now shown. History entries that differ only
  // by fragment (the skip link) must not trigger a search.
  let shownKey = location.pathname + location.search;

  function announce(text) {
    if (!announcer) {
      return;
    }
    // Clear first so the same message twice in a row is still announced.
    announcer.textContent = "";
    window.setTimeout(() => {
      announcer.textContent = text;
    }, 100);
  }

  function setLoading(loading) {
    form.classList.toggle("is-loading", loading);
    if (submitLabel) {
      submitLabel.textContent = loading ? submitLabel.dataset.loadingLabel || idleLabel : idleLabel;
    }
    const results = document.getElementById("results");
    if (!results) {
      return;
    }
    if (loading) {
      results.setAttribute("aria-busy", "true");
      if (skeleton && skeleton.content) {
        results.replaceChildren(skeleton.content.cloneNode(true));
      }
      const notice = document.getElementById("notice");
      if (notice) {
        notice.replaceChildren();
      }
    } else {
      results.removeAttribute("aria-busy");
    }
  }

  /** The URL the form would navigate to. */
  function formURL() {
    const url = new URL(form.getAttribute("action") || "/", location.href);
    url.search = new URLSearchParams(new FormData(form)).toString();
    return url.href;
  }

  /**
   * Swaps the regions of a fetched page into this one. With restoreForm the
   * inputs also take the fetched page's values: for back and forward, and
   * for links that start a different search.
   */
  function applyPage(page, restoreForm) {
    for (const id of REGION_IDS) {
      const current = document.getElementById(id);
      const next = page.getElementById(id);
      if (current && next) {
        current.replaceWith(document.importNode(next, true));
      }
    }
    for (const name of Object.keys(fields)) {
      const next = page.getElementById(name);
      if (!next) {
        continue;
      }
      if (restoreForm) {
        fields[name].value = next.getAttribute("value") || "";
        dirty.delete(name);
        updateHint(name);
      }
      if (next.getAttribute("aria-invalid") === "true") {
        fields[name].setAttribute("aria-invalid", "true");
        touched.add(name);
      } else {
        fields[name].removeAttribute("aria-invalid");
        touched.delete(name);
      }
    }
    if (page.title) {
      document.title = page.title;
    }
  }

  /** Moves focus to what changed and announces it. */
  function revealOutcome() {
    const invalid = Object.keys(fields).find((name) => fields[name].getAttribute("aria-invalid") === "true");
    if (invalid) {
      // Its message is read out with it.
      fields[invalid].focus();
      return;
    }
    const heading = document.getElementById("results-heading");
    if (heading) {
      heading.focus();
      const summary = document.querySelector("#results [data-count]");
      if (summary) {
        const count = Number(summary.getAttribute("data-count"));
        announce(count === 1 ? "1 itinerary found" : `${count} itineraries found`);
      } else {
        announce(heading.textContent.trim());
      }
      return;
    }
    const notice = document.getElementById("notice");
    const title = notice && notice.querySelector(".notice__title");
    if (title) {
      notice.focus();
      announce(title.textContent.trim());
    }
  }

  /**
   * Fetches a search page and swaps it in. push adds a history entry;
   * restoreForm copies the fetched page's values into the inputs.
   */
  async function search(url, { push, restoreForm }) {
    if (inFlight) {
      inFlight.abort();
    }
    const controller = new AbortController();
    inFlight = controller;
    setLoading(true);

    let page;
    let finalURL = url;
    try {
      const response = await fetch(url, {
        signal: controller.signal,
        headers: { Accept: "text/html" },
        credentials: "same-origin",
      });
      const type = response.headers.get("Content-Type") || "";
      if (!type.includes("text/html")) {
        throw new Error(`unexpected content type ${type}`);
      }
      finalURL = response.url || url;
      page = new DOMParser().parseFromString(await response.text(), "text/html");
      // Error pages (404, 500) are not the search page: show them for real.
      if (!REGION_IDS.every((id) => page.getElementById(id))) {
        throw new Error("response is not the search page");
      }
    } catch {
      if (!controller.signal.aborted) {
        // Network failure or an unexpected response: let the server render
        // whatever went wrong as a full page.
        window.location.assign(url);
      }
      return;
    }
    if (controller.signal.aborted) {
      // Superseded by a newer search, or the page is being hidden.
      return;
    }
    inFlight = null;

    applyPage(page, restoreForm);
    setLoading(false);
    if (push) {
      history.pushState({ skypath: true }, "", finalURL);
    }
    shownKey = location.pathname + location.search;
    revealOutcome();
  }

  if (canFetch) {
    // Back and forward re-run the search for the restored address, so the
    // page always matches the URL.
    window.addEventListener("popstate", () => {
      if (location.pathname + location.search !== shownKey) {
        search(location.href, { push: false, restoreForm: true });
      }
    });

    // Links that start a search (the empty state's suggestion) stay in the
    // page too. Modified clicks keep their usual meaning.
    document.addEventListener("click", (event) => {
      const link = event.target instanceof Element ? event.target.closest("a[data-search-link]") : null;
      const modified = event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey;
      if (link && !modified && !event.defaultPrevented) {
        event.preventDefault();
        search(link.href, { push: true, restoreForm: true });
      }
    });

    // A page restored from the back/forward cache may have been frozen
    // mid-search, for example when a failed fetch fell back to navigation.
    // Never leave it stuck: load the results for the address shown.
    window.addEventListener("pageshow", (event) => {
      if (event.persisted && form.classList.contains("is-loading")) {
        search(location.href, { push: false, restoreForm: true });
      }
    });
    window.addEventListener("pagehide", () => {
      if (inFlight) {
        inFlight.abort();
      }
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
      return;
    }
    if (canFetch) {
      event.preventDefault();
      search(formURL(), { push: true, restoreForm: false });
    }
  });
})();
