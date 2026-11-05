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
  label: "多语言",
  fieldRows: []
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

  it("passes the list keys and the child field options", () => {
    const [spec] = childSpecs([
      {
        ...base,
        listKeys: true,
        listKeysLabel: " 已填语言 ",
        fieldRows: [
          {
            column: " image_url ",
            label: "图片",
            image: true,
            hidden: false,
            hint: " 不填用主图 "
          },
          {
            column: "badge_text",
            label: "",
            image: false,
            hidden: true,
            hint: ""
          },
          { column: "title", label: "", image: false, hidden: false, hint: "" },
          { column: "", label: "x", image: false, hidden: false, hint: "" }
        ]
      }
    ]);
    expect(spec.listKeys).toBe(true);
    expect(spec.listKeysLabel).toBe("已填语言");
    expect(spec.fields).toEqual([
      {
        column: "image_url",
        label: "图片",
        upload: "image",
        hint: "不填用主图"
      },
      { column: "badge_text", form: false }
    ]);
    const [back] = childModels([spec]);
    expect(back.fieldRows).toEqual([
      {
        column: "image_url",
        label: "图片",
        image: true,
        hidden: false,
        hint: "不填用主图"
      },
      { column: "badge_text", label: "", image: false, hidden: true, hint: "" }
    ]);
  });

  it("round-trips the key source", () => {
    const [table] = childModels(childSpecs([{ ...base, keySource: "table" }]));
    expect(table.keySource).toBe("table");
    const [dict] = childModels(childSpecs([base]));
    expect(dict.keySource).toBe("dict");
  });
});
