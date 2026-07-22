import { accountCard } from "./account-card.js?v=reset-refresh-actions";
import { providerActions } from "./add-menu.js?v=components";
import { emptyState } from "./common.js?v=components";

const providerExternalLinks = {
  codex: [{ href: "https://codexradar.com/", label: "雷达" }],
};

export function providerSection({ providerInfo, accounts, loading, accountState, actions }) {
  const section = document.createElement("section");
  section.className = "provider-section";

  const header = document.createElement("div");
  header.className = "provider-header";
  header.append(providerTitle(providerInfo));
  header.append(
    providerActions({
      providerInfo,
      loading,
      onLogin: () => actions.createLoginTask(providerInfo.id),
      onImportFile: () => actions.importProviderAuthJSONFile(providerInfo.id),
      onImportJSON: () => actions.importProviderAuthJSON(providerInfo.id),
    }),
  );
  section.append(header);

  if (accounts.length === 0) {
    section.append(emptyState("还没有账号", "点击“添加账号”，通过登录或导入 auth.json 开始使用。"));
    return section;
  }

  const grid = document.createElement("div");
  grid.className = "account-grid";
  for (const account of accounts) {
    const state = accountState(account);
    grid.append(
      accountCard({
        account,
        providerInfo,
        loading,
        isRefreshing: state.isRefreshing,
        isResetting: state.isResetting,
        onRefresh: () => actions.refreshAccount(account),
        onActivate: () => actions.activateAccount(account),
        onDelete: () => actions.deleteAccount(account),
        onReset: () => actions.resetAccountRateLimit(account),
        onUpdatePlanExpiration: () => actions.updatePlanExpiration(account),
      }),
    );
  }
  section.append(grid);
  return section;
}

function providerTitle(providerInfo) {
  const wrapper = document.createElement("div");
  wrapper.className = "provider-title";
  const title = document.createElement("h2");
  title.textContent = providerInfo.displayName || providerInfo.id;
  wrapper.append(title);
  const links = providerExternalLinks[providerInfo.id] || [];
  for (const { href, label } of links) {
    const link = document.createElement("a");
    link.className = "provider-title-link";
    link.href = href;
    link.target = "_blank";
    link.rel = "noopener noreferrer";
    link.textContent = label;
    wrapper.append(link);
  }
  return wrapper;
}
