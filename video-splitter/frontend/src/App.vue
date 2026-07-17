<script setup lang="ts">
import VideoSplitter from './components/VideoSplitter.vue'
import { useTheme } from './ui-system/composables/useTheme'

useTheme()
</script>

<template>
  <VideoSplitter/>
</template>

<style>
/* Remove default wails styles for clean UI */
body {
  margin: 0;
  padding: 0;
  background-color: #f0f2f5;
}
</style>
