<template>
  <div class="mapFilterResult container is-fullhd">

    <div class="modal" v-bind:class="{ 'is-active' : exportModalActive}">
      <div
        class="modal-background"
        @click="exportModalActive=false"
      ></div>
      <div class="modal-content box">
                <p class="has-text-centered"><strong>Export Results CSV</strong> &nbsp; &nbsp; </p>
        <div class="flex-container2">
           <div><label>Source fields</label></div>
            <div><input class="is-medium" size="80" v-model="sourceExportFields" placeholder="Comma seperated attribute names"/></div>
        </div>

        <div class="flex-container2">
            <div><label>Mapping fields </label></div>
            <div><input class="is-medium" size="80" v-model="targetExportFields"  placeholder="Comma seperated attribute names" /></div>
        </div>
        <br/>

        <div class="buttons has-addons is-right">
          <button
            class="button is-warning is-medium"
            @click="exportMapping()"
          >Export</button>
        </div>
      </div>
      <button
        class="modal-close is-large"
        aria-label="close"
        @click="exportModalActive=false"
      ></button>
    </div>

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
      exportModalActive: false,
      sourceExportFields: "",
      targetExportFields: "",
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
    exportMapping: function () {

      let csvContent = "data:text/csv;charset=utf-8,";

      let sourceFields = this.sourceExportFields;
      let targetFields = this.targetExportFields;

      if (sourceFields.length == 0) {
        sourceFields = "source";
      } else {
        sourceFields = "source," + sourceFields;
      }
      if (targetFields.length == 0) {
        targetFields = "mapping";
      } else {
        targetFields = "mapping," + targetFields;
      }

      let sourcePaths = sourceFields.split(',');
      let targetPaths = targetFields.split(',')

      for (let i = 0; i < sourcePaths.length; i++) {
        csvContent += sourcePaths[i] + ",";
      }

      for (let i = 0; i < targetPaths.length; i++) {
        csvContent += targetPaths[i] + ",";
      }
      csvContent += "\n";

      for (let i = 0; i < this.app_model.all_map_results[this.resultIndex].length; i++) {

        const res = this.app_model.all_map_results[this.resultIndex][i];
        const resSource = res.source;
        let sourceVals = resSource.identifier + ",";

        for (let j = 1; j < sourcePaths.length; j++) {

          if (resSource["Attributes"] != undefined) {
            let sourceClone = JSON.parse(JSON.stringify(resSource["Attributes"][Object.keys(resSource["Attributes"])[0]])); // gets attributes objects
            for (var k = 0, path = sourcePaths[j].split('.'), len = path.length; k < len; k++) {
              if (sourceClone != undefined) {
                sourceClone = sourceClone[path[k]];
              }
            }
            sourceVals += "" + sourceClone + ","
          } else {
            sourceVals += "undefined,"
          }

        }

        for (let l = 0; l < res.targets.length; l++) {

          const target = res.targets[l];
          let targetVals = target.identifier + ",";

          for (let j = 1; j < targetPaths.length; j++) {

            if (target["Attributes"] != undefined) {
              let targetClone = JSON.parse(JSON.stringify(target["Attributes"][Object.keys(target["Attributes"])[0]])); // gets attributes objects
              for (var k = 0, path = targetPaths[j].split('.'), len = path.length; k < len; k++) {
                if (targetClone != undefined) {
                  targetClone = targetClone[path[k]];
                }
              }
              targetVals += "" + targetClone + ","

            } else {
              targetVals += "undefined,"
            }
          }
          csvContent += sourceVals + targetVals
          csvContent += "\n";

        }
      }


      const data = encodeURI(csvContent);
      const link = document.createElement("a");
      link.setAttribute("href", data);
      let export_name = "export";
      if (this.app_model.queries[this.queryIndex].name && this.app_model.queries[this.queryIndex].name.length > 0) {
        export_name = this.app_model.queries[this.queryIndex].name;
      }
      link.setAttribute("download", export_name + ".csv");
      link.click();
      this.exportModalActive = false;
      this.sourceExportFields = "";
      this.targetExportFields = "";

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
