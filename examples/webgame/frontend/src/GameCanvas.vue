<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from "vue";
import type Phaser from "phaser";
import { createGame } from "./game";
const host = ref<HTMLDivElement>();
let game: Phaser.Game | undefined;
onMounted(() => {
  if (host.value) game = createGame(host.value);
});
onBeforeUnmount(() => {
  game?.destroy(true);
});
</script>
<template>
  <div ref="host" class="game-canvas" data-testid="game-canvas"></div>
</template>
