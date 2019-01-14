<template>
  <div :class="{'resultBox':!mobile,'resultBoxFirst': !parent_sub_res }">

    <div class="legend">
      <template v-if="sub_res.showResults && sub_res.count>0"> &nbsp; {{ sub_res.count.toLocaleString()}} Results for </template>
      <a
        :href='sub_res.url'
        target='_blank'
      >{{ xref_conf[""+sub_res.domain_id+""].name}} {{ sub_res.identifier}} {{ sub_res.specialKeyword }}</a>
      <!-- 		        <a :href='xref_conf[""+sub_res.domain_id].url.replace("£{id}",sub_res.identifier)' target='_blank'><i class="fas fa-external-link-alt fa-1x"></i></a>  -->
      <a
        class="actionIcon icon"
        title="Remove"
        v-show="sub_res.depth>0"
        @click="removeXref(sub_res,parent_sub_res)"
      ><i class="fas fa-trash-alt"></i></a>
      <a
        title="Filter"
        v-show="sub_res.showResults"
        class="actionIcon icon"
        @click="sub_res.filterModalActive=true"
      ><i
          class="fas fa-filter fa-1x"
          style="position:relative;top:2px"
        ></i></a>
      <a
        class="actionIcon icon"
        title="Hide results"
        @click="hideResults(sub_res)"
        v-show="sub_res.showResults"
      ><i class="fas fa-eye-slash"></i></a>
      <a
        class="actionIcon icon"
        title="Show results"
        @click="showResults(sub_res)"
        v-show="!sub_res.showResults"
      ><i class="fas fa-eye"></i></a>
      <a
        class="actionIcon icon"
        title="Show selection list"
        @click="sub_res.treeModal=true"
        v-show="!parent_sub_res"
      ><i class="fas fa-list-ul"></i></a>

    </div>

    <div>
      <div
        class="xrefs"
        v-show="sub_res.showResults"
      >
        <div class="allEntryBox">
          <div
            :id="'sub_res'+sub_res.counter"
            class="flex-container"
          >
            <span v-if="sub_res.count <= 0">No result to display due to the active filters.</span>
            <div
              v-for="entry in sub_res.displayEntries"
              :style="entry.style"
            >
              <p>
                {{ xref_conf[""+entry.domain_id+""].name}}
                <a
                  @click='selectXref(sub_res.identifier,sub_res.domain_id,entry.xref_id,entry.domain_id,sub_res,entry)'
                  v-show="!entry.selected"
                ><i class="fas fa-plus-circle plusi"></i></a>
              </p>
              <a
                class="exlinkcolor"
                :href='entry.url'
                target='_blank'
                :title="entry.title"
              ><small>{{ entry.label }}</small></a>
              </p>
              <p>
                <!-- 						<a :href='xref_conf[""+entry.domain_id].url.replace("£{id}",entry.xref_id)' -->
                <!-- 						   target='_blank'><i class="fas fa-external-link-alt fa-1x"></i></a> -->
              </p>
            </div>
          </div>

          <div
            class="has-text-centered pagingDiv"
            v-show="sub_res.maxClientPage > 0"
          >
            <a
              title="Previous Page"
              v-show="sub_res.clientPage > 0"
              @click="previousPage(sub_res)"
            ><i class="fas fa-arrow-left"></i></a>
            &nbsp; Page {{sub_res.clientPage+1}} of {{sub_res.maxClientPage+1}} &nbsp;
            <a
              title="Next Page"
              v-show="sub_res.maxClientPage > 0 && sub_res.clientPage<sub_res.maxClientPage"
              @click="nextPage(sub_res)"
            ><i class="fas fa-arrow-right"></i></a>
          </div>

        </div>

      </div>

      <div
        class="modal"
        v-bind:class="{ 'is-active' : sub_res.filterModalActive}"
      >
        <div
          class="modal-background"
          @click="sub_res.filterModalActive=false"
        ></div>
        <div class="modal-content box">
          <p class="has-text-centered"><strong>Filter by dataset</strong> &nbsp; &nbsp; </p>
          <p><a @click="selectAllFilter(sub_res)">Select all</a> &nbsp; <a @click="deSelectAllFilter(sub_res)">Select none</a></p>
          <div class="flex-container2">
            <div v-for="domain_count in sub_res.domain_counts">
              <label class="checkbox">
                <input
                  type="checkbox"
                  v-model="domain_count.selected"
                />
                {{ domain_count.filterLabel }}
              </label>
            </div>
          </div>
          <div class="buttons has-addons is-right">
            <button
              :class="{ 'is-loading' : filterLoading,'button':true, 'is-warning':true, 'is-medium':true}"
              @click="applyFilter(sub_res)"
            >Apply</button>
          </div>
        </div>
        <button
          class="modal-close is-large"
          aria-label="close"
          @click="sub_res.filterModalActive=false"
        ></button>
      </div>
    </div>

    <transition-group
      name="list"
      tag="div"
    >
      <div
        v-for="(sel_sub_res ,index) in sub_res.selectedXrefs"
        style="margin-top:10px"
        :key="sel_sub_res.identifier"
      >
        <box-view
          :mobile="mobile"
          :parent_sub_res="sub_res"
          :sub_res="sel_sub_res"
          :xref_conf="xref_conf"
          :app_conf="app_conf"
          :app_model="app_model"
        ></box-view>
      </div>

    </transition-group>

    <!-- if it is root render treeview -->

    <template v-if="!parent_sub_res">

      <div
        class="modal"
        v-bind:class="{ 'is-active' : sub_res.treeModal}"
      >
        <div
          class="modal-background"
          @click="sub_res.treeModal=false"
        ></div>
        <div class="modal-content box">
          <p><strong>Summary view</strong></p>
          <p class="tree"> <a
              :href='xref_conf[""+sub_res.domain_id].url.replace("£{id}",sub_res.identifier)'
              target='_blank'
            >{{ xref_conf[""+sub_res.domain_id+""].name}} {{ sub_res.identifier}}</a></p>
          <ul class="tree">
            <template v-for="sel_sub_res in sub_res.selectedXrefs">
              <tree-view
                :sel_sub_res="sel_sub_res"
                :xref_conf="xref_conf"
                :app_conf="app_conf"
              ></tree-view>
            </template>
          </ul>
        </div>
        <button
          class="modal-close is-large"
          aria-label="close"
          @click="sub_res.treeModal=false"
        ></button>
      </div>
    </template>

  </div>

</template>
<script>
import TreeView from "./TreeView.vue";

export default {
  name: "box-view",
  components: {
    "tree-view": TreeView
  },
  props: {
    sub_res: {
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
      results: null,
      testclass: "dfsdfsa",
      lastSelectedRes: null,
      xrefSelected: false,
      filterLoading: false
    };
  },
  methods: {
    processSelectedXref: function(results, callback_params) {
      this.lastSelectedRes = this.app_model.processSelectedXref(
        results,
        callback_params
      );
      this.xrefSelected = true;
    },
    selectXref: function(
      id,
      domain_id,
      entry_id,
      entry_domain_id,
      sub_res,
      entry
    ) {
      let callback_params = [
        id,
        domain_id,
        entry_id,
        entry_domain_id,
        sub_res,
        entry
      ];
      this.$root.$data.fetcher.search(
        entry_id,
        this.processSelectedXref.bind(this),
        callback_params
      );
    },
    processFilteredResults: function(
      data_results,
      sub_result,
      callback_params,
      fail
    ) {
      sub_result.selectedXrefs = [];
      sub_result.filterModalActive = false;
      this.filterLoading = false;
      if (fail) {
        this.$notify({
          group: "error",
          title: "",
          text: "Something went wrong :("
        });
        return;
      }

      this.app_model.processFilteredResults(data_results, sub_result, false);
    },
    processFilteredResults4Paging: function(
      data_results,
      sub_result,
      callback_params,
      fail
    ) {
      sub_result.selectedXrefs = [];
      sub_result.filterModalActive = false;

      if (fail) {
        this.$notify({
          group: "error",
          title: "",
          text: "Something went wrong :("
        });
        return;
      }

      this.app_model.processFilteredResults(data_results, sub_result, true);
      sub_result.displayEntries = sub_result.entries.slice(
        callback_params[0],
        callback_params[1]
      );
    },
    applyFilter: function(sub_result) {
      this.filterLoading = true;
      let domain_counts = sub_result.domain_counts;
      let filters = "";
      let filterSet = new Set();
      for (var key in domain_counts) {
        let domain_count = domain_counts[key];
        if (domain_count.selected) {
          filters += domain_count.domain_id + ",";
          filterSet.add(domain_count.domain_id);
        }
      }
      //TODO check if all selected then it means not filtering.
      sub_result.filters = filters;
      sub_result.hasFilter = true;
      sub_result.lastFilteredPageKey = null;
      let callback_params = [];
      this.$root.$data.fetcher.searchByFilter(
        sub_result,
        this.processFilteredResults.bind(this),
        callback_params
      );
    },
    processPagingResults: function(results, callback_params) {
      this.app_model.processPagingResults(results, callback_params[0]);
      callback_params[0].displayEntries = callback_params[0].entries.slice(
        callback_params[1],
        callback_params[2]
      );
    },
    setDisplayEntries(sub_result) {
      let start = sub_result.clientPage * this.app_conf.page_size;
      let end =
        sub_result.clientPage * this.app_conf.page_size +
        this.app_conf.page_size;

      if (end > sub_result.count) {
        end = sub_result.count;
      }
      if (end <= sub_result.entries.length) {
        sub_result.displayEntries = sub_result.entries.slice(start, end);
      } else if (end <= sub_result.count) {
        // fetch more data from server.
        if (sub_result.hasFilter) {
          let callback_params = [start, end];
          this.$root.$data.fetcher.searchByFilter(
            sub_result,
            this.processFilteredResults4Paging.bind(this),
            callback_params
          );
        } else {
          let callback_params = [sub_result, start, end];
          this.$root.$data.fetcher.searchByPageIndex(
            sub_result.identifier,
            sub_result.domain_id,
            sub_result.serverPage,
            sub_result.maxServerPage,
            this.processPagingResults.bind(this),
            callback_params
          );
        }
      }
    },
    previousPage(sub_result) {
      sub_result.clientPage = sub_result.clientPage - 1;
      this.setDisplayEntries(sub_result);
    },
    nextPage(sub_result) {
      sub_result.clientPage = sub_result.clientPage + 1;
      this.setDisplayEntries(sub_result);
    },
    removeXref(sub_res, parent_sub_res) {
      if (!parent_sub_res) {
        return;
      }

      for (let key in parent_sub_res.selectedXrefs) {
        let s = parent_sub_res.selectedXrefs[key];
        if (
          s.identifier === sub_res.identifier &&
          s.domain_id === sub_res.domain_id
        ) {
          parent_sub_res.selectedXrefs.splice(key, 1);
          break;
        }
      }

      for (let key in parent_sub_res.entries) {
        let entry = parent_sub_res.entries[key];
        if (
          sub_res.identifier === entry.xref_id &&
          entry.domain_id === sub_res.domain_id
        ) {
          entry.selected = false;
          entry.style["background-color"] = this.app_conf.box_color;
        }
      }
    },

    hideResults(sub_res) {
      sub_res.showResults = false;
    },
    showResults(sub_res) {
      sub_res.showResults = true;
    },

    selectAllFilter(sub_res) {
      for (let key in sub_res.domain_counts) {
        let domain_count = sub_res.domain_counts[key];
        domain_count.selected = true;
      }
    },

    deSelectAllFilter(sub_res) {
      for (let key in sub_res.domain_counts) {
        let domain_count = sub_res.domain_counts[key];
        domain_count.selected = false;
      }
    }
  },
  updated() {
    if (this.xrefSelected) {
      var d = document.getElementById("sub_res" + this.lastSelectedRes.counter);
      this.$scrollTo(d);
      this.xrefSelected = false;
    }
  }
};
</script>
<style scoped>
.allEntryBox {
  border: solid 1px teal;
  padding-left: 3px;
}
.actionIcon {
  color: steelblue;
}
.list-item {
  display: inline-block;
  margin-right: 10px;
}
.list-enter-active,
.list-leave-active {
  transition: all 0.7s;
  /**background-color: green;**/
}
.list-enter, .list-leave-to /* .list-leave-active below version 2.1.8 */ {
  opacity: 0;
  transform: translateY(30px);
}

div.last {
  animation: blink 0.5s ease;
}

@keyframes blink {
  0% {
    background-color: green;
  }
}

.resultBox {
  margin-left: 10px;
}

.resultBoxFirst {
  margin-left: 0 !important;
}

.flex-container {
  display: flex;
  flex-wrap: wrap;
}

.xrefs {
  border: 1px solid #efefef;
  margin-bottom: 15px;
}

.flex-container > div {
  width: 128px;
  height: 80px;
  margin: 10px;
  padding-top: 10px;
  padding-bottom: 10px;
  line-height: 2;
  text-align: center;
  /** line-height: 75px;
  font-size: 30px;**/
}

.flex-container2 {
  display: flex;
  flex-wrap: wrap;
}

.flex-container2 > div {
  width: 160px;
  heigh: 10px;
  margin: 10px;
  text-align: left;
  /** line-height: 75px;
  font-size: 30px;**/
}

.legend {
  text-align: left;
}
.fieldset {
  border-color: antiquewhite;
}
.plusi {
  font-size: 12px;
}
.exlinkcolor {
  color: black;
}

a[target^="_blank"]:after {
  /** content: url(http://upload.wikimedia.org/wikipedia/commons/6/64/Icon_External_Link.png);**/
  content: url(data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAYAAACNMs+9AAAAVklEQVR4Xn3PgQkAMQhDUXfqTu7kTtkpd5RA8AInfArtQ2iRXFWT2QedAfttj2FsPIOE1eCOlEuoWWjgzYaB/IkeGOrxXhqB+uA9Bfcm0lAZuh+YIeAD+cAqSz4kCMUAAAAASUVORK5CYII=);
  margin: 0 0 0 1px;
}

.pagingDiv {
  padding-bottom: 5px;
}
</style>
