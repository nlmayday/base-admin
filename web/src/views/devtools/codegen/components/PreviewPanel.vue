<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { CodegenPreview } from "@/api/devtools/codegen";
import CodeBlock from "../../shared/CodeBlock.vue";

defineOptions({ name: "DevtoolsCodegenPreview" });

const props = defineProps<{ preview: CodegenPreview }>();

const activeFile = ref("");
const existing = computed(() => props.preview.files.filter(f => f.exists));

watch(
  () => props.preview,
  p => {
    activeFile.value = p.files[0]?.path ?? "";
  },
  { immediate: true }
);

function basename(path: string): string {
  return path.slice(path.lastIndexOf("/") + 1);
}
</script>

<template>
  <div>
    <div class="mb-3 flex flex-wrap items-center gap-2">
      <el-tag :type="preview.newModule ? 'success' : 'info'">
        {{ preview.newModule ? "新模块" : "已有模块" }}
      </el-tag>
      <span class="text-sm text-[var(--el-text-color-secondary)]">
        共 {{ preview.files.length }} 个文件，{{
          preview.permissions.length
        }}
        个权限码，{{ preview.menus.length }} 个菜单
      </span>
    </div>
    <el-alert
      v-if="existing.length > 0"
      type="warning"
      :closable="false"
      show-icon
      class="mb-3"
      :title="`${existing.length} 个文件已存在，CLI 需加 -force 才会覆盖`"
      :description="existing.map(f => f.path).join('，')"
    />

    <el-tabs v-model="activeFile" type="card">
      <el-tab-pane
        v-for="f in preview.files"
        :key="f.path"
        :name="f.path"
        :label="basename(f.path)"
      >
        <template #label>
          <span>{{ basename(f.path) }}</span>
          <el-tag
            v-if="f.exists"
            size="small"
            type="warning"
            effect="plain"
            class="ml-1"
          >
            已存在
          </el-tag>
        </template>
        <CodeBlock
          :code="f.content"
          :language="f.language"
          :title="f.path"
          max-height="560px"
        />
      </el-tab-pane>
    </el-tabs>

    <div class="mt-4 grid grid-cols-1 gap-3 xl:grid-cols-2">
      <div class="min-w-0">
        <div class="mb-2 font-medium">权限码</div>
        <el-table :data="preview.permissions" border size="small">
          <el-table-column prop="code" label="编码" min-width="200">
            <template #default="{ row }">
              <span class="font-mono">{{ row.code }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="name" label="名称" min-width="120" />
        </el-table>
        <div class="mt-3 mb-2 font-medium">菜单</div>
        <el-table :data="preview.menus" border size="small">
          <el-table-column prop="code" label="编码" min-width="140" />
          <el-table-column label="类型" width="70" align="center">
            <template #default="{ row }">
              <el-tag size="small" effect="plain">
                {{ row.type === "dir" ? "目录" : "页面" }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="title" label="标题" min-width="100" />
          <el-table-column
            prop="component"
            label="组件"
            min-width="160"
            show-overflow-tooltip
          />
        </el-table>
      </div>
      <div class="min-w-0">
        <div class="mb-2 font-medium">接入步骤</div>
        <CodeBlock
          v-if="preview.newModule"
          :code="preview.wiring"
          language="go"
          title="server/internal/app/modules.go"
          :line-numbers="false"
        />
        <ul class="notes mt-3">
          <li v-for="n in preview.notes" :key="n">{{ n }}</li>
        </ul>
      </div>
    </div>
  </div>
</template>

<style scoped>
.notes {
  padding-left: 18px;
  font-size: 13px;
  line-height: 1.8;
  color: var(--el-text-color-regular);
  list-style: disc;
}
</style>
