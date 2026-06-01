type TextareaValues = Record<string, string | undefined>;

export const activeInputRegistry = [
  { type: "privateLinks", textareaId: "in_private_links", exportFile: "private-links.json" },
  { type: "firewalls", textareaId: "in_firewalls", exportFile: "firewalls.json" },
];

export const uploadFileMap = {
  "private-links.json": "in_private_links",
  "firewalls.json": "in_firewalls",
};

export const snapshotDiffIds = {
  privateLinks: "in_private_endpoints",
  firewalls: "in_firewalls",
};

function parseJsonList(raw: string | undefined) {
  if (!raw) return [];
  const parsed = JSON.parse(raw);
  return Array.isArray(parsed) ? parsed : [];
}

export function buildSnapshotContext(textareas: TextareaValues) {
  return {
    privateLinks: parseJsonList(textareas[snapshotDiffIds.privateLinks]),
    firewalls: parseJsonList(textareas[snapshotDiffIds.firewalls]),
  };
}
