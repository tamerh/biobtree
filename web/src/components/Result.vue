<template>
  <div>
  <div class="resultContainer container is-fullhd" v-if="app_model.queries.length>0">

    <div class="querybox" v-if="app_model.queries.length>1">
      <div class="field is-grouped is-grouped-multiline">

      <div class="control" v-for="(query,index) in app_model.queries">
        <div class="tags has-addons">
          <a :class="{'tag':true,'is-link':index==selectedQueryIndex}" @click="selectQuery(index)" v-show="!app_model.queries[index].edit" @dblclick="enableQueryEdit(index)" title="double click to name the query">{{queryLabel(index)}}</a>
          <input class="is-small" v-model="app_model.queries[index].name" v-show="app_model.queries[index].edit" @blur="disableQueryEdit(index)" v-on:keyup.enter="disableQueryEdit(index)"/>
          <a class="tag is-delete" @click="deleteQuery(index)"></a>
        </div>
      </div>

      </div>
    </div>
   <div class="mapfilter" v-if="app_model.queries[selectedQueryIndex].type==1"> <!-- MAPFILTER BOX -->
    <div class="control has-icons-left">
      <p class="control is-expanded field has-addons">
        <input
          class="input is-normal control"
          ref="searchbox"
          type="search"
          maxlength="30000"
          placeholder="Terms"
          v-model="app_model.queries[selectedQueryIndex].searchTerm"
          v-on:keyup.enter="mapFilter"
        />
      <span class="icon is-normal is-left">
        <i class="fa fa-search"></i>
      </span>
      </p>
    </div>

      <div>
        <multiselect
          :multiple="false"
          v-model="app_conf.global_filter_datasets_new"
          track-by="id"
          label="name"
          placeholder="Type at least 3 letters e.g uniprot"
          :options="options"
          :searchable="true"
          @search-change="findDataset"
          open-direction="bottom"
          :internal-search="false"
          @select="onDatasetSelect"
          :show-no-results="false"
          :show-no-options="false"
          v-show="this.app_model.queries[this.selectedQueryIndex].showDatasets"
          :reset-after="true">
          <template slot="singleLabel" slot-scope="{ option }">
            <strong>{{ option.name }}</strong>
          </template>
        </multiselect>
      </div>


    <div class="control has-icons-left"> 
          <p class="control is-expanded field has-addons">
            <input
              class="input is-normal control"
              ref="searchbox"
              type="search"
              maxlength="300"
              placeholder="MapFilter query"
              v-model="app_model.queries[selectedQueryIndex].mapFilterTerm"
              v-on:keyup.enter="mapFilter"
            />
            <span class="icon is-normal is-left">
              <i class="fa fa-map-signs"></i>
            </span>
            <span class="control">
            <a 
            :class="{ 'is-loading' : app_model.queries[selectedQueryIndex].loading,'button':true, 'is-success':true, 'is-normal':true}"
            @click="mapFilter">Map</a>
          </span>
          </p>
      </div>

      <div class="actions" v-show="app_model.queries[selectedQueryIndex].retrieved">
        <div>
          <span><label class="checkbox"><input type="checkbox" v-model="app_model.queries[selectedQueryIndex].attributes"> Attributes</label></span>
          <span><label class="checkbox"><input type="checkbox" v-model="app_model.queries[selectedQueryIndex].showDatasets">Set dataset <label class="has-text-info has-text-weight-bold" v-show="app_model.queries[this.selectedQueryIndex].selectedDatasetName.length>0">{{this.app_model.queries[this.selectedQueryIndex].selectedDatasetName}}</label></label></span>
          <span><label class="checkbox"><input type="checkbox" v-model="showUrl">REST url</label></span>
        </div>

        <div v-show="showUrl">
          <a class="exlinkcolor" :href='app_model.queries[selectedQueryIndex].restURL' target='_blank'>
            {{app_model.queries[selectedQueryIndex].restURL}}
          </a>
        </div>

        <div>
          <a v-show="app_model.queries[selectedQueryIndex].nextPageKey && app_model.queries[selectedQueryIndex].nextPageKey.length>0" :class="{ 'is-loading' : nextLoading, 'is-normal':true,'is-pulled-right':true}" @click="mapFilterMore">Load More result</a>
        </div>
      </div>
</div>    
    
     <div class="search" v-else> <!-- SEARCH BOX -->

      <div class="control has-icons-left">
        <p class="control is-expanded field has-addons">
          <input
            class="input is-normal control"
            ref="searchbox"
            type="search"
            placeholder="Search"
            :size="topSearchBoxSize"
            v-model="app_model.queries[selectedQueryIndex].searchTerm"
            v-on:keyup.enter="newSearch"
            @keyup="searchKeyUp"
            maxlength="300"
          />
          <span class="icon is-normal is-left">
            <i class="fa fa-search"></i>
          </span>
          <span class="control">
              <a 
              :class="{ 'is-loading' :app_model.queries[selectedQueryIndex].loading ,'button':true, 'is-info':true, 'is-normal':true}"
              @click="newSearch">Search</a>
          </span>
        </p>
      </div>
      <div class="control has-icons-left" v-if="app_model.queries[selectedQueryIndex].filterActive">
        <p class="is-expanded field has-addons">
          <input
            class="input is-normal control"
            ref="filter"
            type="search"
            placeholder='Add filter e.g ensembl.genome=="homo_sapiens"'
            :size="topSearchBoxSize"
            v-model="app_model.queries[selectedQueryIndex].filter"
            v-on:keyup.enter="search"
            @keyup="searchKeyUp"
            maxlength="300"
          />
          <span class="icon is-normal is-left">
            <i class="fa fa-filter"></i>
          </span>
        </p>
      </div>

      <div>
        <multiselect
          :multiple="false"
          v-model="app_conf.global_filter_datasets_new"
          track-by="id"
          label="name"
          placeholder="Type at least 3 letters e.g uniprot"
          :options="options"
          :searchable="true"
          @search-change="findDataset"
          open-direction="bottom"
          :internal-search="false"
          @select="onDatasetSelect"
          :show-no-results="false"
          :show-no-options="false"
          v-show="this.app_model.queries[this.selectedQueryIndex].showDatasets"
          :reset-after="true">
          <template slot="singleLabel" slot-scope="{ option }">
            <strong>{{ option.name }}</strong>
          </template>
        </multiselect>
      </div>
      

    <div class="actions" v-show="app_model.queries[selectedQueryIndex].retrieved">
      <div>
        <span><label class="checkbox"><input type="checkbox" v-model="app_model.queries[selectedQueryIndex].filterActive">Add filter</label></span>
        <span><label class="checkbox"><input type="checkbox" v-model="app_model.queries[selectedQueryIndex].showDatasets">Set dataset <label class="has-text-info has-text-weight-bold" v-show="app_model.queries[this.selectedQueryIndex].selectedDatasetName.length>0">{{this.app_model.queries[this.selectedQueryIndex].selectedDatasetName}}</label></label></span> 
        <span><label class="checkbox"><input type="checkbox" v-model="showUrl">REST url</label></span>
      </div>
      <div v-show="showUrl">
        <span>
          <a class="exlinkcolor" :href='app_model.queries[selectedQueryIndex].restURL' target='_blank'>
            {{app_model.queries[selectedQueryIndex].restURL}}
          </a>
        </span>
      </div>
      <div>
        <a v-if="app_model.queries[selectedQueryIndex] && app_model.queries[selectedQueryIndex].nextPageKey && app_model.queries[selectedQueryIndex].nextPageKey.length>0" 
          class="is-pulled-right is-normal" @click="search">Load More result</a>
      </div>
    </div>

    </div>
 </div>

  <search-main
    :mobile="mobile"
    :xref_conf="xref_conf"
    :app_conf="app_conf"
    :app_model="app_model"
    ref="searchComp"
    v-show="app_model.queries[selectedQueryIndex] && app_model.queries[selectedQueryIndex].type==0">
  </search-main>

  <map-filter
      :mobile="mobile"
      :xref_conf="xref_conf"
      :app_conf="app_conf"
      :app_model="app_model"
      ref="mapFilterComp"
      v-show="app_model.queries[selectedQueryIndex] && app_model.queries[selectedQueryIndex].type==1">
  </map-filter>


  </div>
</template>

<script>
import VueJsonPretty from "vue-json-pretty";
import MapFilter from "./MapFilter.vue";
import Search from "./Search.vue";

export default {
  name: "biobtree-result",
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
      topSearchBoxSize: 50,
      nextLoading: false,
      restUrl: "",
      showUrl: false,
      selectedQueryIndex: 0,
      options: []
    };
  },
  components: {
    VueJsonPretty,
    "map-filter": MapFilter,
    "search-main": Search,
  },
  methods: {
    findDataset: function (query) {
      if (query.length >= 3) {
        this.options = [];
        for (let key in this.xref_conf) {
          if (!this.xref_conf[key].linkdataset && this.xref_conf[key].name.toLowerCase().includes(query.toLowerCase())) {
            this.options.push({ id: this.xref_conf[key].id, name: this.xref_conf[key].name });
          }
        }
        this.options.sort(function (a, b) {
          var nameA = a.name.toLowerCase(),
            nameB = b.name.toLowerCase();
          if (nameA < nameB) return -1;
          if (nameA > nameB) return 1;
          return 0;
        });
      } else {
        this.options = [];
      }
    },
    onDatasetSelect: function (option) {
      this.app_model.queries[this.selectedQueryIndex].selectedDataset = option.id;
      this.app_model.queries[this.selectedQueryIndex].selectedDatasetName = option.name;
      this.app_model.queries[this.selectedQueryIndex].showDatasets = false;
      this.options = [];

      if (this.app_model.queries[this.selectedQueryIndex].type == 0) {
        this.search();
      } else if (this.app_model.queries[this.selectedQueryIndex].type == 1) {
        this.mapFilter();
      }

    },
    search: function () {
      if (this.app_model.queries[this.selectedQueryIndex].searchTerm.length == 0) {
        return false;
      }
      if (this.app_model.queries[this.selectedQueryIndex].searchTerm.length == 1) {
        this.$notify({
          group: "xrefmap",
          title: "",
          text: "Query must be at least 2 characters"
        });
        return false;
      }
      if (this.$refs.searchbox) {
        this.$refs.searchbox.blur();
      }
      this.app_model.queries[this.selectedQueryIndex].loading = true;

      if (this.app_model.queries[this.selectedQueryIndex].searchTerm.startsWith("alias:")) {
        var alias = this.app_model.queries[this.selectedQueryIndex].searchTerm.split("alias:")[1];
        if (alias.length <= 1) {
          this.$notify({
            group: "appwarn",
            title: "",
            text: "Query must be at least 2 characters"
          });
        }
      }

      this.$refs.searchComp.search(this.selectedQueryIndex);

    },
    mapFilter: function () {
      if (this.app_model.queries[this.selectedQueryIndex].searchTerm.length == 0) {
        return false;
      }
      if (this.app_model.queries[this.selectedQueryIndex].mapFilterTerm.length == 0) {
        return false;
      }
      this.app_model.queries[this.selectedQueryIndex].loading = true;
      this.app_model.queries[this.selectedQueryIndex].nextPageKey = "";
      this.$refs.mapFilterComp.mapFilter(this.selectedQueryIndex);
    },
    mapFilterMore: function () {
      this.app_model.queries[this.selectedQueryIndex].loading = true;
      this.$refs.mapFilterComp.mapFilter(this.selectedQueryIndex);
    },
    resultDivClass: function (index) {
      if (index % 2 == 0) {
        return "resultDivOdd";
      } else {
        return "resultDivEven";
      }
    },
    newQuery: function (type) {
      this.app_model.newQuery(type, "", "", "")
      this.selectedQueryIndex = this.app_model.queries.length - 1;
      if (type == 0) {
        this.$refs.searchComp.selectQuery(this.selectedQueryIndex);
      } else if (type == 1) {
        this.$refs.mapFilterComp.selectQuery(this.selectedQueryIndex);
      }

    },
    mapFilterHasResult: function () {
      let resultIndex = this.selectedQueryIndex - this.app_model.previousSearchQueryCount(this.selectedQueryIndex);
      if (this.app_model.all_map_results[resultIndex] && this.app_model.all_map_results[resultIndex].length > 0) {
        return true;
      }
      return false;
    },
    searchHasResult: function () {
      let resultIndex = this.selectedQueryIndex - this.app_model.previousMapQueryCount(this.selectedQueryIndex);
      if (this.app_model.all_sub_results[resultIndex] && this.app_model.all_sub_results[resultIndex].length > 0) {
        return true;
      }
      return false;
    },
    selectQuery: function (index) {
      this.selectedQueryIndex = index;
      if (this.app_model.queries[index].type == 0) {
        this.$refs.searchComp.selectQuery(this.selectedQueryIndex);
      } else if (this.app_model.queries[index].type == 1) {
        this.$refs.mapFilterComp.selectQuery(this.selectedQueryIndex);
      }
    },
    reset: function () {
      this.selectedQueryIndex = 0;
    },
    newSearch() {
      this.app_model.queries[this.selectedQueryIndex].nextPageKey = "";
      this.search();
    },
    deleteQuery: function (index) {

      this.app_model.deleteQuery(index);
      if (this.selectedQueryIndex >= index) {
        this.selectedQueryIndex = this.selectedQueryIndex - 1;
      }

    },
    queryLabel: function (index) {
      if (this.app_model.queries[index].name.length > 0) {
        return this.app_model.queries[index].name;
      } else {
        return "" + (index + 1);
      }
    },
    enableQueryEdit: function (index) {
      this.app_model.queries[index].edit = true;
    },
    disableQueryEdit: function (index) {
      this.app_model.queries[index].edit = false;
    },
    searchKeyUp: function () {
      if (this.app_model.queries[this.selectedQueryIndex].searchTerm.length > 0) {
        this.showExample = false;
      } else {
        this.showExample = true;
      }
    }
  }
};
</script>
<style src="vue-multiselect/dist/vue-multiselect.min.css"></style>
<style scoped>
a[target^="_blank"]:after {
  /** content: url(http://upload.wikimedia.org/wikipedia/commons/6/64/Icon_External_Link.png);**/
  content: url(data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAYAAACNMs+9AAAAVklEQVR4Xn3PgQkAMQhDUXfqTu7kTtkpd5RA8AInfArtQ2iRXFWT2QedAfttj2FsPIOE1eCOlEuoWWjgzYaB/IkeGOrxXhqB+uA9Bfcm0lAZuh+YIeAD+cAqSz4kCMUAAAAASUVORK5CYII=);
  margin: 0 0 0 1px;
}
.labelmf {
  padding-top: 10px;
  padding-right: 3px;
  font-size: 0.8rem;
}
.querybox {
  padding-bottom: 0.5rem;
}
.actions {
  display: flex; /* establish flex container */
  flex-direction: row; /* default value; can be omitted */
  flex-wrap: nowrap; /* default value; can be omitted */
  justify-content: space-between; /* switched from default (flex-start, see below) */
  /* background-color: lightyellow; */
  border-bottom: 1px solid #dbdbdb;
  border-left: 1px solid #dbdbdb;
  border-right: 1px solid #dbdbdb;
}
.actions > div {
  margin: 3px;
}
.actions > div > span {
  font-size: 0.875em;
  padding: 0.5em;
}
</style>
