<template>
<div>
<div class="resultTitle" v-if="app_model.all_sub_results[resultIndex] && app_model.all_sub_results[resultIndex].length>0">Results</div>
<div v-for="(sub_res,index) in app_model.all_sub_results[resultIndex]" :class="resultDivClass(index)" v-if="app_model.all_sub_results[resultIndex]">
  <div class="resultContainer container is-fullhd">
    <box-view
      :mobile="mobile"
      :sub_res="sub_res"
      :xref_conf="xref_conf"
      :app_conf="app_conf"
      :app_model="app_model"
    ></box-view>
  </div>
</div>

</div>
</template>

<script>
import BoxView from "./BoxView.vue";
export default {
  name: "search-main",
  props: {
    xref_conf: {
      type: Object,
      required: true
    },
    app_conf: {
      type: Object,
      required: true
    },
    app_model: {
      type: Object,
      required: true
    },
    mobile: {
      type: Boolean,
      required: true
    }
  },
  data() {
    return {
      searchResultActive: false,
      topSearchBoxSize: 50,
      searchLoading: false,
      nextLoading: false,
      nextPageKey: "",
      restUrl: "",
      showUrl: false,
      resultIndex: 0
    };
  },
  components: {
    "box-view": BoxView
  },
  methods: {
    resultDivClass: function (index) {
      if (index % 2 == 0) {
        return "resultDivOdd";
      } else {
        return "resultDivEven";
      }
    },
    processResults: function (results, callback_params) {
      this.app_model.queries[callback_params[0]].nextPageKey = this.app_model.processResults(results, callback_params[0]);
      this.app_model.queries[callback_params[0]].restURL = callback_params[1];
      this.app_model.queries[callback_params[0]].loading = false;
      this.app_model.queries[callback_params[0]].retrieved = true;
    },

    search: function (qindex) {
      this.app_model.queries[qindex].loading = true;
      this.$root.$data.fetcher.search(
        this.app_model.queries[qindex].searchTerm,
        this.app_model.queries[qindex].nextPageKey,
        this.app_model.queries[qindex].filter,
        this.app_model.queries[qindex].selectedDataset,
        this.processResults.bind(this),
        [qindex, ""]
      );
    },
    selectQuery: function (index) {
      this.resultIndex = index - this.app_model.previousMapQueryCount(index);
      if (this.app_model.queries[index].searchTerm.length > 0 && !this.app_model.queries[index].retrieved) {
        this.search(index);
      }
    },
    reset: function () {
      this.resultIndex = 0;
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
.resultTitle {
  text-align: center;
  vertical-align: middle;
  color: firebrick;
  font-weight: bold;
  margin-top: 4px;
  padding-right: 85px;
}
</style>
