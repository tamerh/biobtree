<template>
  <li>
    <a
      :href="xref_conf[""+sel_sub_res.domain_id].url.replace("£{id}",sel_sub_res.identifier)"
      target="_blank"
    >{{ xref_conf[""+sel_sub_res.domain_id+""].name}} {{ sel_sub_res.identifier}}</a>
    <ul v-if="sel_sub_res.selectedXrefs.length>0">
      <template v-for="sel_sub_res2 in sel_sub_res.selectedXrefs">
        <tree-view
          :sel_sub_res="sel_sub_res2"
          :xref_conf="$root.$data.xref_conf"
          :app_conf="$root.$data.app_conf"
        ></tree-view>
      </template>
    </ul>
  </li>
</template>

<script>
export default {
  name: "tree-view",
  props: {
    sel_sub_res: {
      type: Object,
      required: true
    },
    parent_sub_res: {
      type: Object
    },
    xref_conf: {
      type: Object,
      required: true
    },
    app_conf: {
      type: Object,
      required: true
    }
  },
  data() {
    return {};
  },
  methods: {}
};
</script>

<style scoped>
a[target^="_blank"]:after {
  /** content: url(http://upload.wikimedia.org/wikipedia/commons/6/64/Icon_External_Link.png);**/
  content: url(data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAYAAACNMs+9AAAAVklEQVR4Xn3PgQkAMQhDUXfqTu7kTtkpd5RA8AInfArtQ2iRXFWT2QedAfttj2FsPIOE1eCOlEuoWWjgzYaB/IkeGOrxXhqB+uA9Bfcm0lAZuh+YIeAD+cAqSz4kCMUAAAAASUVORK5CYII=);
  margin: 0 0 0 1px;
}
</style>
