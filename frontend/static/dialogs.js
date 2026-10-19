import { isValidDateInput } from "./formatters.js?v=single-plan-date";

let dialogSequence = 0;

export function promptPlanDialog(options) {
  const fields = document.createElement("div");
  fields.className = "dialog-fields";

  const renewalGroup = document.createElement("fieldset");
  renewalGroup.className = "dialog-renewal-group";
  const legend = document.createElement("legend");
  legend.textContent = "自动续期";
  const choices = document.createElement("div");
  choices.className = "dialog-renewal-options";
  renewalGroup.append(legend, choices);

  const initialRenewal = options.autoRenew === false ? "false" : "true";
  const radios = [];
  for (const [value, text] of [["true", "是"], ["false", "否"]]) {
    const label = document.createElement("label");
    label.className = "dialog-radio-option";
    const radio = document.createElement("input");
    radio.type = "radio";
    radio.name = "planAutoRenew";
    radio.value = value;
    radio.checked = value === initialRenewal;
    label.append(radio, document.createTextNode(text));
    choices.append(label);
    radios.push(radio);
  }

  const dateField = document.createElement("div");
  dateField.className = "dialog-field";
  const dateLabel = document.createElement("label");
  dateLabel.htmlFor = "plan-date";
  const dateInput = document.createElement("input");
  dateInput.id = "plan-date";
  dateInput.name = "planDate";
  dateInput.type = "date";
  dateInput.min = "1970-01-02";
  dateInput.max = "9999-12-31";
  dateInput.autocomplete = "off";
  dateInput.value = options.date || "";
  const hint = document.createElement("p");
  hint.id = "plan-date-hint";
  hint.className = "dialog-hint";
  hint.textContent = "日期留空可清除；续期后请手动更新。";
  dateInput.setAttribute("aria-describedby", hint.id);
  const selectedRenewal = () => radios.find((radio) => radio.checked).value;
  const updateDateLabel = () => {
    const value = selectedRenewal();
    dateLabel.textContent = value === "true" ? "续期日期" : "到期日期";
  };
  renewalGroup.addEventListener("change", updateDateLabel);
  updateDateLabel();
  dateField.append(dateLabel, dateInput, hint);
  fields.append(renewalGroup, dateField);

  return new Promise((resolve) => {
    openFormDialog({
      title: "套餐信息",
      detail: options.detail || "",
      body: fields,
      submitText: "保存",
      initialFocus: radios.find((radio) => radio.checked),
      validate: () => (!dateInput.value || isValidDateInput(dateInput.value) ? "" : "日期格式无效"),
      onSubmit: () => resolve({
        planAutoRenew: selectedRenewal() === "true",
        date: dateInput.value,
      }),
      onCancel: () => resolve(null),
    });
  });
}

export function promptAuthJSON(options = {}) {
  const textarea = document.createElement("textarea");
  textarea.name = "authJson";
  textarea.rows = 14;
  textarea.autocomplete = "off";
  textarea.spellcheck = false;
  textarea.placeholder = '{\n  "tokens": {}\n}';
  return new Promise((resolve) => {
    openFormDialog({
      title: "导入 auth.json",
      detail: options.detail || "",
      body: textarea,
      submitText: "导入",
      initialFocus: textarea,
      validate: () => (textarea.value.trim() ? "" : "auth.json 内容不能为空"),
      onSubmit: () => resolve(textarea.value.trim()),
      onCancel: () => resolve(null),
    });
  });
}

export function promptAuthJSONFile(options = {}) {
  const input = document.createElement("input");
  input.name = "authJsonFile";
  input.type = "file";
  input.accept = ".json,application/json";
  return new Promise((resolve) => {
    openFormDialog({
      title: "导入 auth.json 文件",
      detail: options.detail || "请选择不超过 2 MiB 的 JSON 文件。",
      body: input,
      submitText: "导入文件",
      initialFocus: input,
      validate: () => {
        const file = input.files && input.files[0];
        if (!file) {
          return "请选择 JSON 文件";
        }
        if (file.size === 0) {
          return "JSON 文件内容不能为空";
        }
        if (file.size > 2 * 1024 * 1024) {
          return "JSON 文件不能超过 2 MiB";
        }
        return "";
      },
      onSubmit: () => resolve(input.files[0]),
      onCancel: () => resolve(null),
    });
  });
}

export function promptTextDialog(options) {
  const input = document.createElement("input");
  input.name = options.fieldName;
  input.type = options.inputType || "text";
  input.autocomplete = options.autocomplete || "off";
  input.placeholder = options.placeholder || "";
  input.value = options.initialValue || "";
  return new Promise((resolve) => {
    openFormDialog({
      title: options.title,
      detail: options.detail || "",
      body: input,
      submitText: options.submitText || "确定",
      initialFocus: input,
      validate: () => {
        const value = input.value.trim();
        if (options.validate) {
          return options.validate(value);
        }
        return "";
      },
      onSubmit: () => resolve(input.value.trim()),
      onCancel: () => resolve(null),
    });
  });
}

export function confirmDialog(options) {
  const detail = document.createElement("div");
  detail.className = "dialog-detail";
  if (options.detailContent) {
    detail.append(options.detailContent);
  } else {
    detail.textContent = options.detail || "";
  }
  return new Promise((resolve) => {
    openFormDialog({
      title: options.title,
      body: detail,
      submitText: options.confirmText || "确定",
      submitDanger: options.danger || false,
      onSubmit: () => resolve(true),
      onCancel: () => resolve(false),
    });
  });
}

function openFormDialog(options) {
  const dialog = document.createElement("dialog");
  dialog.className = "app-dialog";
  const form = document.createElement("form");
  form.method = "dialog";

  const title = document.createElement("h2");
  title.id = `app-dialog-title-${++dialogSequence}`;
  title.textContent = options.title;
  dialog.setAttribute("aria-labelledby", title.id);
  form.append(title);

  if (options.detail) {
    const detail = document.createElement("p");
    detail.className = "dialog-account";
    detail.textContent = options.detail;
    form.append(detail);
  }

  form.append(options.body);

  const error = document.createElement("p");
  error.className = "dialog-error";
  error.setAttribute("aria-live", "polite");
  error.hidden = true;
  form.append(error);

  const actions = document.createElement("div");
  actions.className = "dialog-actions";
  const cancelButton = document.createElement("button");
  cancelButton.type = "button";
  cancelButton.textContent = "取消";
  const submitButton = document.createElement("button");
  submitButton.type = "submit";
  submitButton.textContent = options.submitText || "确定";
  if (options.submitDanger) {
    submitButton.classList.add("danger");
  }
  actions.append(cancelButton, submitButton);
  form.append(actions);

  dialog.append(form);
  document.body.append(dialog);

  let settled = false;
  const close = (cancelled) => {
    if (settled) {
      return;
    }
    settled = true;
    dialog.close();
    dialog.remove();
    if (cancelled && options.onCancel) {
      options.onCancel();
    }
  };
  cancelButton.addEventListener("click", () => close(true));
  dialog.addEventListener("cancel", (event) => {
    if (event.target !== dialog) {
      return;
    }
    event.preventDefault();
    close(true);
  });
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const message = options.validate ? options.validate() : "";
    if (message) {
      error.textContent = message;
      error.hidden = false;
      if (options.initialFocus) {
        options.initialFocus.focus();
      }
      return;
    }
    if (settled) {
      return;
    }
    settled = true;
    dialog.close();
    dialog.remove();
    if (options.onSubmit) {
      options.onSubmit();
    }
  });
  dialog.showModal();
  const initialFocus = options.initialFocus || dialog.querySelector("button[type='submit']");
  if (initialFocus) {
    initialFocus.focus();
  }
}
