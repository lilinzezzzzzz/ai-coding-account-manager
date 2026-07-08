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
  const detail = document.createElement("p");
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
  title.textContent = options.title;
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
