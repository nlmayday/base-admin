import { describe, expect, it } from "vitest";
import {
  DEPT_TREE_PROPS,
  collectSubtreeIds,
  flattenTree,
  markDisabled,
  treeSelectProps
} from "./tree-options";

const tree = [
  {
    id: "1",
    name: "总公司",
    children: [
      { id: "2", name: "研发部", children: [{ id: "3", name: "前端组" }] },
      { id: "4", name: "市场部" }
    ]
  }
];

describe("tree options", () => {
  it("collects a node and all its descendants", () => {
    expect([...collectSubtreeIds(tree, "2")].sort()).toEqual(["2", "3"]);
    expect(collectSubtreeIds(tree, "9").size).toBe(0);
  });

  it("marks disabled nodes without touching the input", () => {
    const marked = markDisabled(tree, new Set(["2", "3"]));
    expect(marked[0].disabled).toBe(false);
    expect(marked[0].children[0].disabled).toBe(true);
    expect(marked[0].children[0].children[0].disabled).toBe(true);
    expect("disabled" in tree[0]).toBe(false);
  });

  it("flattens breadth first", () => {
    expect(flattenTree(tree).map(n => n.id)).toEqual(["1", "2", "4", "3"]);
  });

  it("builds el-tree-select props with the value key", () => {
    expect(DEPT_TREE_PROPS).toEqual({
      value: "id",
      label: "name",
      children: "children"
    });
    expect(treeSelectProps("title", { disabled: "disabled" })).toEqual({
      value: "id",
      label: "title",
      children: "children",
      disabled: "disabled"
    });
  });
});
