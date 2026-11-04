import { describe, expect, it } from "vitest";
import { childModels, childSpecs, type ChildModel } from "./advanced";

const base: ChildModel = {
  table: "lg_item_i18n",
  foreignKey: "item_id",
  keyColumn: "lang",
  keySource: "dict",
  keyDict: "lang",
  keyTable: "lg_language",
  keyValueColumn: "code",
  keyLabelColumn: "name",
  keyFilter: " enabled = 1 ",
  keySortColumn: "",
  keyRequiredColumn: "required",
  label: "多语言"
};

describe("childSpecs", () => {
  it("keeps the options of the chosen key source only", () => {
    expect(childSpecs([base])).toEqual([
      {
        table: "lg_item_i18n",
        foreignKey: "item_id",
        keyColumn: "lang",
        keyDict: "lang",
        label: "多语言"
      }
    ]);
    expect(childSpecs([{ ...base, keySource: "table" }])).toEqual([
      {
        table: "lg_item_i18n",
        foreignKey: "item_id",
        keyColumn: "lang",
        label: "多语言",
        keyTable: "lg_language",
        keyValueColumn: "code",
        keyLabelColumn: "name",
        keyFilter: "enabled = 1",
        keyRequiredColumn: "required"
      }
    ]);
    expect(childSpecs([{ ...base, table: "" }])).toEqual([]);
  });

  it("round-trips the key source", () => {
    const [table] = childModels(childSpecs([{ ...base, keySource: "table" }]));
    expect(table.keySource).toBe("table");
    const [dict] = childModels(childSpecs([base]));
    expect(dict.keySource).toBe("dict");
  });
});
