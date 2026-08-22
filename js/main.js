const burger = document.getElementById("burger");
const nav = document.getElementById("nav");
const modal = document.getElementById("modal");
const toast = document.getElementById("toast");
const calcForm = document.getElementById("calcForm");
const leadForm = document.getElementById("leadForm");
const modalForm = document.getElementById("modalForm");
const faqMore = document.getElementById("faqMore");
const faqList = document.getElementById("faqList");
const quizFields = ["platforms", "category", "dailyVolume"];

const state = {
  platforms: "Ozon",
  category: "Бытовая химия",
  dailyVolume: "От 2000",
};

function toggleNav(force) {
  const open = typeof force === "boolean" ? force : !nav.classList.contains("is-open");
  nav.classList.toggle("is-open", open);
  burger.setAttribute("aria-expanded", String(open));
  burger.setAttribute("aria-label", open ? "Закрыть меню" : "Открыть меню");
}

burger.addEventListener("click", () => toggleNav());

nav.querySelectorAll("a").forEach((link) => {
  link.addEventListener("click", () => toggleNav(false));
});

function otherInput(form, name) {
  return form.querySelector(`[name="${name}Other"], [data-other-for="${name}"]`);
}

function toggleOther(input, show) {
  if (!input) return;
  input.hidden = !show;
  input.required = show;
  if (!show) input.value = "";
  if (show) input.focus();
}

function optionExists(select, value) {
  return Array.from(select.options).some((option) => option.value === value);
}

function setChoice(form, name, value) {
  const select = form[name];
  const other = otherInput(form, name);
  if (!select) return;

  if (value && value !== "Другое" && optionExists(select, value)) {
    select.value = value;
    toggleOther(other, false);
    return;
  }

  if (value) {
    select.value = "Другое";
    toggleOther(other, true);
    if (other && value !== "Другое") other.value = value;
    return;
  }

  toggleOther(other, select.value === "Другое");
}

function resolveChoice(form, name) {
  const select = form[name];
  if (!select) return "";
  const value = select.value.trim();
  if (value !== "Другое") return value;
  const custom = (otherInput(form, name)?.value || "").trim();
  return custom || "Другое";
}

document.querySelectorAll(".pills").forEach((group) => {
  group.addEventListener("click", (event) => {
    const pill = event.target.closest(".pill");
    if (!pill) return;
    group.querySelectorAll(".pill").forEach((item) => item.classList.remove("is-active"));
    pill.classList.add("is-active");
    const name = group.dataset.group;
    const value = pill.dataset.value;
    state[name] = value;
    const other = calcForm.querySelector(`[data-other-for="${name}"]`);
    toggleOther(other, value === "Другое");
  });
});

calcForm.querySelectorAll("[data-other-for]").forEach((input) => {
  input.addEventListener("input", () => {
    const name = input.dataset.otherFor;
    const text = input.value.trim();
    state[name] = text || "Другое";
  });
});

[leadForm, modalForm].forEach((form) => {
  quizFields.forEach((name) => {
    const select = form[name];
    if (!select) return;
    select.addEventListener("change", () => {
      toggleOther(otherInput(form, name), select.value === "Другое");
    });
  });
});

function maskPhone(input) {
  input.addEventListener("input", () => {
    const digits = input.value.replace(/\D/g, "").replace(/^8/, "7").slice(0, 11);
    let value = "+7";
    if (digits.length > 1) value += " (" + digits.slice(1, 4);
    if (digits.length >= 4) value += ") " + digits.slice(4, 7);
    if (digits.length >= 7) value += "-" + digits.slice(7, 9);
    if (digits.length >= 9) value += "-" + digits.slice(9, 11);
    input.value = digits.length ? value : "";
  });

  input.addEventListener("focus", () => {
    if (!input.value) input.value = "+7 (";
  });
}

document.querySelectorAll('input[type="tel"]').forEach(maskPhone);

function isValidPhone(value) {
  const digits = value.replace(/\D/g, "");
  return digits.length === 11 && digits.startsWith("7");
}

function showToast(message) {
  toast.textContent = message;
  toast.hidden = false;
  window.clearTimeout(showToast.timer);
  showToast.timer = window.setTimeout(() => {
    toast.hidden = true;
  }, 2800);
}

function fillModalQuiz(values = {}) {
  quizFields.forEach((name) => setChoice(modalForm, name, values[name] || state[name]));
  if (values.city) modalForm.city.value = values.city;
}

function openModal(options = {}) {
  modal.hidden = false;
  fillModalQuiz(options);
  document.body.style.overflow = "hidden";
  window.setTimeout(() => modalForm.name.focus(), 0);
}

function closeModal() {
  modal.hidden = true;
  document.body.style.overflow = "";
}

document.querySelectorAll("[data-open-modal]").forEach((el) => {
  el.addEventListener("click", () => openModal());
});

document.querySelectorAll("[data-close-modal]").forEach((el) => {
  el.addEventListener("click", closeModal);
});

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape" && !modal.hidden) closeModal();
});

function missingOther(form) {
  for (const name of quizFields) {
    const select = form[name];
    if (!select || select.value !== "Другое") continue;
    const other = otherInput(form, name);
    if (!other || !other.value.trim()) return other || select;
  }
  return null;
}

calcForm.addEventListener("submit", (event) => {
  event.preventDefault();
  const city = calcForm.city.value.trim();
  if (!city) {
    calcForm.city.focus();
    return;
  }
  const emptyOther = calcForm.querySelector("[data-other-for]:not([hidden])");
  if (emptyOther && !emptyOther.value.trim()) {
    emptyOther.focus();
    showToast("Напишите свой вариант в поле «Другое»");
    return;
  }
  openModal({
    city,
    platforms: state.platforms,
    category: state.category,
    dailyVolume: state.dailyVolume,
  });
});

function collectLead(form) {
  return {
    name: form.name.value.trim(),
    phone: form.phone.value.trim(),
    platforms: resolveChoice(form, "platforms"),
    category: resolveChoice(form, "category"),
    city: form.city.value.trim(),
    dailyVolume: resolveChoice(form, "dailyVolume"),
  };
}

async function sendLead(payload) {
  const body = JSON.stringify(payload);
  const headers = { "Content-Type": "application/json" };
  const endpoints = ["/api/send-lead.php"];
  let lastCode = "send_failed";

  for (const url of endpoints) {
    try {
      const response = await fetch(url, { method: "POST", headers, body });
      const data = await response.json().catch(() => ({}));
      if (response.ok && data.ok) return;
      if (data.error) lastCode = data.error;
    } catch (err) {
      lastCode = "send_failed";
    }
  }

  const error = new Error(lastCode);
  error.code = lastCode;
  throw error;
}

async function handleLeadSubmit(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const emptyOther = missingOther(form);
  if (emptyOther) {
    emptyOther.focus();
    showToast("Напишите свой вариант в поле «Другое»");
    return;
  }

  const lead = collectLead(form);

  if (!lead.name) {
    form.name.focus();
    return;
  }
  if (!lead.city) {
    form.city.focus();
    return;
  }
  if (!isValidPhone(lead.phone)) {
    form.phone.focus();
    showToast("Введите телефон в формате +7 (900) 000-00-00");
    return;
  }

  const button = form.querySelector('button[type="submit"]');
  const previousText = button ? button.textContent : "";
  if (button) {
    button.disabled = true;
    button.textContent = "Отправляем…";
  }

  try {
    await sendLead(lead);
    form.reset();
    quizFields.forEach((name) => toggleOther(otherInput(form, name), false));
    closeModal();
    showToast("Заявка отправлена. Мы свяжемся с вами.");
  } catch (error) {
    console.error(error);
    showToast("Ошибка сервера. Попробуйте ещё раз позже.");
  } finally {
    if (button) {
      button.disabled = false;
      button.textContent = previousText;
    }
  }
}

leadForm.addEventListener("submit", handleLeadSubmit);
modalForm.addEventListener("submit", handleLeadSubmit);

faqMore.addEventListener("click", () => {
  const expanded = faqList.classList.toggle("is-expanded");
  faqMore.textContent = expanded ? "Скрыть" : "Показать ещё";
});

document.getElementById("passportLink").addEventListener("click", (event) => {
  event.preventDefault();
  showToast("Паспорт качества отправим на почту после заявки");
});
