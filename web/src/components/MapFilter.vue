<template>
  <div class="mapFilterResult container is-fullhd">
    <div class="resultTitle" v-if="app_model.all_map_results[resultIndex] && app_model.all_map_results[resultIndex].length>0">Results</div>
    <table class="table is-bordered is-narrow is-hoverable is-fullwidth"  v-if="app_model.all_map_results[resultIndex] && app_model.all_map_results[resultIndex].length>0">
      <thead>
        <tr>
          <th></th>
          <th>Source</th>
          <th>Mapping</th>
        </tr>
      </thead>
      <tbody v-for="(sub_res,index) in app_model.all_map_results[resultIndex]">
        <tr v-for="(res,index2) in sub_res.targets">
          <td>{{index2+1}}</td>
          <td>
            <template v-if="index2>0">〃</template>
            <template v-if="index2==0">{{xref_conf[sub_res.source.dataset].name}} - </template>
            <template v-if="!app_model.queries[queryIndex].attributes && index2==0 && sub_res.source.keyword && sub_res.source.keyword.length>0">{{sub_res.source.keyword}} - </template>
            <template v-if="!app_model.queries[queryIndex].attributes && index2==0">{{sub_res.source.identifier}}</template>
            <template v-if="app_model.queries[queryIndex].attributes && index2==0">
              <a :href="sub_res.source.url" target="_blank" v-if="sub_res.source.url && sub_res.source.url.length>0">{{ sub_res.source.identifier}}</a>
              <label v-else>{{ sub_res.source.identifier}}</label>
              <div v-show="res && !sub_res.source.Attributes.Empty && app_model.queries[queryIndex].attributes">
                <vue-json-pretty
                  :path="'res'"
                  :data="sub_res.source.Attributes"
                  :showDoubleQuotes="false"
                  :deep="2"
                ></vue-json-pretty>
              </div>
            </template>

          </td>
          <td>
            <template v-if="!app_model.queries[queryIndex].attributes">{{res.identifier}}</template>
            <template v-if="app_model.queries[queryIndex].attributes">
              <a :href="res.url" target="_blank" v-if="res.url && res.url.length>0">{{ res.identifier}}</a>
              <label v-else>{{ res.identifier}}</label>
              <div v-show="res && !res.Attributes.Empty && app_model.queries[queryIndex].attributes">
                <vue-json-pretty
                  :path="'res'"
                  :data="res.Attributes"
                  :showDoubleQuotes="false"
                  :deep="2"
                ></vue-json-pretty>
              </div>
            </template>
          </td>
        </tr>
      </tbody>
    </table>

  </div>
</template>

<script>
import VueJsonPretty from "vue-json-pretty";

export default {
  name: "map-filter",
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
      mapFilterActive: false,
      nextLoading: false,
      restUrl: "",
      showUrl: false,
      resultIndex: 0,
      queryIndex: 0
    };
  },
  components: {
    VueJsonPretty,
  },
  methods: {
    processMapFilter: function (results, callback_params) {
      this.app_model.queries[callback_params[0]].nextPageKey = this.app_model.processMPResults(results, callback_params[0]);
      this.app_model.queries[callback_params[0]].restURL = callback_params[1];
      this.app_model.queries[callback_params[0]].loading = false;
      this.app_model.queries[callback_params[0]].retrieved = true;
    },
    mapFilter: function (qindex) {
      this.queryIndex = qindex;
      this.app_model.queries[qindex].searchTerm = this.app_model.queries[qindex].searchTerm.trim();
      this.app_model.queries[qindex].mapFilterTerm = this.app_model.queries[qindex].mapFilterTerm.trim();

      if (this.app_model.queries[qindex].searchTerm.length == 0 || this.app_model.queries[qindex].mapFilterTerm.length == 0) {
        return false;
      }
      if (this.app_model.queries[qindex].searchTerm.length == 1) {
        return false;
      }

      //var mUri = "./?s=" + encodeURIComponent(this.app_model.queries[qindex].searchTerm) + "&m=" + encodeURIComponent(this.app_model.queries[qindex].mapFilterTerm);
      //history.pushState("", "page", mUri);
      let callback_params = [qindex, ""];
      this.mapFilterActive = true;
      if (this.app_model.queries[qindex].searchTerm.startsWith("alias:")) {
        var alias = this.app_model.queries[qindex].searchTerm.split("alias:")[1];
        if (alias.length <= 1) {
          return false;
        }
        this.app_model.queries[qindex].loading = true;
        this.$root.$data.fetcher.mapFilter(
          this.app_model.queries[qindex].searchTerm,
          this.app_model.queries[qindex].mapFilterTerm,
          this.app_model.queries[qindex].nextPageKey,
          this.app_model.queries[qindex].selectedDataset,
          this.processMapFilter.bind(this),
          callback_params)
      } else {
        this.app_model.queries[qindex].loading = true;
        this.$root.$data.fetcher.mapFilter(
          this.app_model.queries[qindex].searchTerm,
          this.app_model.queries[qindex].mapFilterTerm,
          this.app_model.queries[qindex].nextPageKey,
          this.app_model.queries[qindex].selectedDataset,
          this.processMapFilter.bind(this),
          callback_params)
      }

    },
    resultDivClass: function (index) {
      if (index % 2 == 0) {
        return "resultDivOdd";
      } else {
        return "resultDivEven";
      }
    },
    selectQuery: function (index) {
      this.queryIndex = index;
      this.resultIndex = index - this.app_model.previousSearchQueryCount(index);
      if (this.app_model.queries[index].searchTerm.length > 0 && this.app_model.queries[index].mapFilterTerm.length > 0 && !this.app_model.queries[index].retrieved) {
        this.mapFilter(index);
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
.mapFilterResult {
  margin-top: 4px;
}
</style>
