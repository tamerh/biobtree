<template>
  <div
    class="hero-body is-fullheight"
    v-show="aboutActive"
  >
    <div class="container">
      <div class="content">
        <p class="has-text-centered is-size-4 has-text-success">About</p>
        <p>
          More information and some stats will come to this page. For any issue or error please contact from github
          <a
            href="https://www.github.com/tamerh/biobtree"
            target="_blank"
          >page</a>.
        </p>
      </div>
    </div>
  </div>
</template>

<script>
export default {
  name: "About",
  props: {
    aboutActive: {
      type: Boolean,
      required: true
    }
  }
};
</script>
 
<style scoped>
a[target^="_blank"]:after {
  /** content: url(http://upload.wikimedia.org/wikipedia/commons/6/64/Icon_External_Link.png);**/
  content: url(data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAYAAACNMs+9AAAAVklEQVR4Xn3PgQkAMQhDUXfqTu7kTtkpd5RA8AInfArtQ2iRXFWT2QedAfttj2FsPIOE1eCOlEuoWWjgzYaB/IkeGOrxXhqB+uA9Bfcm0lAZuh+YIeAD+cAqSz4kCMUAAAAASUVORK5CYII=);
  margin: 0 0 0 1px;
}
</style>
