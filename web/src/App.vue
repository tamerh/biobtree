<template>
  <div id="app">
    <header>
      <nav class="navbar has-shadow" id="topnav">
        <div class="container">
          <div class="navbar-brand">
            <div class="navbar-item">
              <h1 v-show="!mainPageActive" class="title is-size-4 logoColor2" @click="goToMain()" style="cursor:pointer">biobtree</h1>
            </div> 
            <a  class="navbar-burger burger" aria-label="menu" aria-expanded="false" @click="burgerBarActive = !burgerBarActive" data-target="navMenu">
              <span aria-hidden="true"></span>
              <span aria-hidden="true"></span>
              <span aria-hidden="true"></span>
            </a>
          </div>

        <div id="navMenu" :class="{'navbar-menu':true,'is-active':burgerBarActive}" style="margin-right:-0.5rem">
          
          <div class="navbar-start" style="flex-grow: 1; justify-content: center;">
              <label class="pageTitle title is-size-5" v-if="resultActive"> 
                <div>
                <span style="padding-right:5px;"><a class="button is-primary is-small is-outlined" @click="newSearchQuery()">
                  <span>Search</span>
                  <span class="icon is-small">
                    <i class="fas fa-plus"></i>
                  </span>
                  </a>
                </span>
                <span><a class="button is-info is-small is-outlined" @click="newMfQuery()">
                  <span>Mapping</span>
                  <span class="icon is-small">
                    <i class="fas fa-plus"></i>
                  </span>
                </a> 
                </span>
                </div>
              </label>
                   
              <h1 class="pageTitle title is-size-5" v-if="settingsActive">Settings</h1>
          </div>

          <div class="navbar-end">
            <a v-show="!mainPageActive" class="navbar-item" @click="goToSettings">Settings</a>
            <div class="navbar-item">
              <div class="field is-grouped">
                <p class="control">
                  <a class="bd-tw-button button" target="_blank" href="https://github.com/tamerh/biobtree">
                    <span class="icon"><i class="fab fa-github"></i></span>
                    <span>Github</span>
                  </a>
                </p>
              </div>
            </div>
          </div>
           </div>

        </div>
      </nav>
    </header>

    <main class="mainContent">
      <!-- Hero content: will be in the middle -->
      <div class="hero-body is-fullheight" v-show="mainPageActive">
        <div class="container has-text-centered">
          <h1 class="title is-size-1 logoColor" style="padding-bottom:10px;">biobtree</h1>
          <h2 class="subtitle"></h2>

          <div class="columns is-gapless">
            <div class="column"></div>
            <div class="column is-four-fifths ">
              <div class="control has-icons-left has-icons-right">
                <p class="control is-expanded field has-addons">
                 
                  <input
                    class="input is-medium control"
                    type="search"
                    :placeholder="searchPlaceHolder"
                    v-model="searchTerm"
                    v-on:keyup.enter="search"
                    @blur="showExample=true"
                    @keyup="searchKeyUp"
                    maxlength="10000"
                    autofocus
                  />
                  <span class="icon is-medium is-left">
                    <i class="fa fa-search"></i>
                  </span>
                
                 <span class="control">
                  <a 
                  :class="{ 'is-loading' : searchLoading,'button':true, 'is-info':true, 'is-medium':true}"
                  @click="search">Search</a>
                </span>
                

                </p>
                 
              </div>
            </div>
            <div class="column"></div>
          </div>

          <br />
          <div class="columns is-gapless">
            <div class="column">
              <p class="logoColor title is-size-5">Apply Chain Mappings</p>
            </div>
          </div>

          <div class="columns is-gapless">
            <div class="column"></div>
            <div class="column is-four-fifths">
              <div class="control has-icons-left has-icons-right">
                <p class="control is-expanded field has-addons">
                  <input
                    class="input is-medium control"
                    type="search"
                    :placeholder="mapFilterPlaceHolder"
                    v-model="mapFilterTerm"
                    v-on:keyup.enter="mapFilter"
                    @blur="showExample=true"
                    @keyup="mapFilterKeyUp"
                    maxlength="300"
                    autofocus
                  />
                  <span class="icon is-medium is-left">
                    <i class="fa fa-map-signs"></i>
                  </span>
                 <span class="control">
                  <a 
                  :class="{ 'is-loading' : mapFilterLoading,'button':true, 'is-success':true, 'is-medium':true}"
                  @click="mapFilter">Map</a>
                </span>
                </p>
              </div>
            </div>
            <div class="column"></div>
          </div>

          <usecase v-on:catusecases="execCatUseCases" v-on:usecase="execUseCase" :usecases="usecases"></usecase>
          
        </div>
      </div>

      <bulk :bulkActive="bulkActive" :xref_conf="xref_conf" v-on:close-bulk="bulkActive=false" />

      <about :aboutActive="aboutActive" v-on:close-about="aboutActive=false" />
      <api :apiActive="apiActive" v-on:close-api="apiActive=false" />
      <settings
        :settingsActive="settingsActive"
        :app_conf="app_conf"
        :xref_conf="xref_conf"
        v-on:apply-settings="applySettings()"
      />

      <biobtree-result
          :mobile="mobile"
          :xref_conf="xref_conf"
          :app_conf="app_conf"
          :app_model="app_model"
          ref="resultComp">
      </biobtree-result>


      <notifications group="appwarn" position="top center" classes="notification is-warning" />
      <notifications group="error" position="top center"   classes="notification is-danger" />
      <notifications group="success" position="top center" classes="notification is-success" />
    
    </main>

    <footer>
      <div class="content has-text-left is-size-6">
        <!-- <p>
          Last update date: 8 October 2018
        </p>-->
      </div>
    </footer>
  </div>
</template>
  
<script>
import About from "./components/About.vue";
import Usecase from "./components/Usecase.vue";
import Bulk from "./components/Bulk.vue";
import Api from "./components/Api.vue";
import Settings from "./components/Settings.vue";
import Result from "./components/Result.vue";

export default {
  name: "App",
  props: {
    app_model: {
      type: Object,
      required: true
    },
    xref_conf: {
      type: Object,
      required: true
    },
    app_conf: {
      type: Object,
      required: true
    },
    usecases: {
      type: Object,
      required: true
    }
  },
  components: {
    "usecase": Usecase,
    "biobtree-result": Result,
    about: About,
    bulk: Bulk,
    api: Api,
    settings: Settings
  },
  data() {
    return {
      searchTerm: "",
      mapFilterTerm: "",
      showExample: true,
      aboutActive: false,
      bulkActive: false,
      resultActive: false,
      settingsActive: false,
      apiActive: false,
      mainPageActive: true,
      topSearchBoxSize: 70,
      burgerBarActive: false,
      searchPlaceHolder:
        "Search identifiers or special keywords like gene name with comma seperated e.g  vav_human,shh",
      mapFilterPlaceHolder: 'Apply chain MapFilters e.g map(go).filter(go.type=="biological_process")',
      mobile: false,
      nextPageKey: "",
      searchLoading: false,
      mapFilterLoading: false
    };
  },
  methods: {
    goToMain: function () {
      this.searchTerm = "";
      this.mapFilterTerm = "";
      this.settingsActive = false;
      this.aboutActive = false;
      this.mainPageActive = true;
      this.bulkActive = false;
      this.resultActive = false;
      this.app_model.reset();
      this.$refs.resultComp.reset();
      history.pushState("", "page", "./?m");
    },
    goToBulk: function () {
      this.searchTerm = "";
      this.mainPageActive = false;
      this.settingsActive = false;
      this.aboutActive = false;
      this.bulkActive = true;
      this.app_model.reset();
      this.$refs.resultComp.reset();
      history.pushState("", "page", "./?b");
    },
    goToAbout: function () {
      this.searchTerm = "";
      this.mainPageActive = false;
      this.settingsActive = false;
      this.aboutActive = true;
      this.bulkActive = false;
      this.app_model.reset();
      this.$refs.resultComp.reset();
      history.pushState("", "page", "./?a");
    },
    goToSettings: function () {
      this.searchTerm = "";
      this.mainPageActive = false;
      this.aboutActive = false;
      this.settingsActive = true;
      this.bulkActive = false;
      this.resultActive = false;
      this.app_model.reset();
      this.$refs.resultComp.reset();
      history.pushState("", "page", "./?s");
    },

    notifyUser: function (status, msg) {
      switch (status) {
        case -1:
          this.$notify({
            group: "error",
            title: "",
            text: msg,
            duration: 5000
          });
          break;
        case 0:
          this.$notify({
            group: "appwarn",
            title: "",
            text: msg,
            duration: 5000
          });
          break;
        case 1:
          this.$notify({
            group: "success",
            title: "",
            text: msg
          });
        default:
          this.$notify({
            group: "appwarn",
            title: "",
            text: msg,
            duration: 5000
          });
          break;
      }
    },
    mapFilter: function () {
      if (this.validQuery2()) {
        var mUri = "./?s=" + encodeURIComponent(this.searchTerm) + "&m=" + encodeURIComponent(this.mapFilterTerm);
        history.pushState("", "page", mUri);
        this.mapFilterLoading = true;
        this.mainPageActive = false;
        this.settingsActive = false;
        this.aboutActive = false;
        this.bulkActive = false;
        this.app_model.freshMapFilterQuery(this.searchTerm, this.mapFilterTerm);
        this.$refs.resultComp.reset();
        this.resultActive = true;
        this.$refs.resultComp.mapFilter();
        this.mapFilterLoading = false;

      }
    },
    search: function () {
      if (this.validQuery()) {
        history.pushState("", "page", "./?s=" + encodeURIComponent(this.searchTerm));
        this.searchLoading = true;
        this.mainPageActive = false;
        this.settingsActive = false;
        this.aboutActive = false;
        this.bulkActive = false;
        this.app_model.freshSearchQuery(this.searchTerm);
        this.$refs.resultComp.reset();
        this.resultActive = true;
        this.$refs.resultComp.search();
        this.searchLoading = false;
      }
    },

    validQuery: function () {
      this.searchTerm = this.searchTerm.trim();
      if (this.searchTerm.length == 0) {
        return false;
      }
      if (this.searchTerm.length == 1) {
        this.notifyUser(0, "Query must be at least 2 characters");
        return false;
      }
      return true;
    },
    searchNoHistory: function () {
      if (this.validQuery()) {
        //this.$refs.searchComp.searchFromMain(this.searchTerm, [0]);
        //this.mainPageActive = false;
      }
    },
    mapNoHistory: function () {
      if (this.validQuery()) {
        this.app_model.mapFilter(this.searchTerm, this.mapFilterTerm);
        this.mainPageActive = false;
      }
    },
    validQuery2: function () {
      this.searchTerm = this.searchTerm.trim();
      if (this.searchTerm.length == 0) {
        return false;
      }
      if (this.searchTerm.length == 1) {
        this.notifyUser(0, "Query must be at least 2 characters");
        return false;
      }

      if (this.mapFilterTerm == 0) {
        return false;
      }
      return true;
    },
    searchKeyUp: function () {
      if (this.searchTerm.length > 0) {
        this.showExample = false;
      } else {
        this.showExample = true;
      }
    },
    mapFilterKeyUp: function () {
      if (this.searchTerm.length > 0) {
        this.showExample = false;
      } else {
        this.showExample = true;
      }
    },
    popStateChange: function (e) {

      this.notifyUser(0, "Back button not supported at the moment please click biobtree to go the main page");
      return;

      let search = document.location.search;
      if (search.length > 4) {
        let searchParams = new URLSearchParams(window.location.search);
        this.searchTerm = searchParams.get("s");
        if (searchParams.get("m") != null && searchParams.get("m").length > 0) {
          this.mapFilterTerm = searchParams.get("m");
          this.mapNoHistory();
        } else {
          this.searchNoHistory();
        }
        this.mainPageActive = false;
        this.settingsActive = false;
        this.aboutActive = false;
        this.bulkActive = false;
      } else if (search === "?m") {
        this.searchTerm = "";
        this.app_model.reset();
        this.mainPageActive = true;
        this.settingsActive = false;
        this.aboutActive = false;
        this.bulkActive = false;
      } else if (search === "?a") {
        this.searchTerm = "";
        this.app_model.reset();
        this.mainPageActive = false;
        this.settingsActive = false;
        this.aboutActive = true;
      } else if (search === "?s") {
        this.searchTerm = "";
        this.app_model.reset();
        this.mainPageActive = false;
        this.settingsActive = true;
        this.aboutActive = false;
        this.bulkActive = false;
      } else if (search === "?b") {
        this.searchTerm = "";
        this.app_model.reset();
        this.settingsActive = false;
        this.aboutActive = false;
        this.bulkActive = true;
      } else {
        this.searchTerm = "";
        this.app_model.reset();
        this.mainPageActive = true;
        this.settingsActive = false;
        this.aboutActive = false;
      }
    },
    applySettings: function () {
      let new_page_value = parseInt(this.app_conf.page_size_new);

      if (new_page_value != this.app_conf.page_size) {
        this.app_conf.page_size = new_page_value;
        this.app_model.resetPaging();
      }

      //if(this.app_conf.global_filter_datasets !== this.app_conf.global_filter_datasets_new){
      this.app_conf.global_filter_datasets = this.app_conf.global_filter_datasets_new;
      this.app_model.setGlobHasFilter(
        this.app_conf.global_filter_datasets &&
        this.app_conf.global_filter_datasets.length > 0
      );
      //}

      let colorChanged = false;

      if (this.app_conf.box_color_new !== this.app_conf.box_color) {
        this.app_conf.box_color = this.app_conf.box_color_new;
        colorChanged = true;
      }

      if (
        this.app_conf.selected_box_color_new !==
        this.app_conf.selected_box_color
      ) {
        this.app_conf.selected_box_color = this.app_conf.selected_box_color_new;
        colorChanged = true;
      }

      if (colorChanged) {
        this.app_model.resetBoxColors();
      }

      this.notifyUser(1, "Settings applied.");

    },
    useCaseQuery: function (query) {
      this.searchTerm = query;
      this.search();
    },
    newMfQuery: function () {
      this.$refs.resultComp.newQuery(1);
    },
    newSearchQuery: function () {
      this.$refs.resultComp.newQuery(0);
    },
    mapFilterActive: function () {
      if (this.$refs.mapFilterComp) {
        return this.$refs.mapFilterComp.mapFilterActive;
      }
      return false;
    },
    searchResultActive: function () {

      if (this.$refs.searchComp) {
        return this.$refs.searchComp.searchResultActive;
      }
      return false;

    },
    execUseCase: function (usecase) {

      if (usecase.type == 0) {
        this.searchTerm = usecase.searchTerm;
        this.search();
      } else if (usecase.type == 1) {
        this.searchTerm = usecase.searchTerm;
        this.mapFilterTerm = usecase.mapFilterTerm;
        this.mapFilter();
      }

    },
    execCatUseCases: function (catusecases) {

      if (!catusecases && catusecases.length <= 0) {
        return;
      }

      this.searchLoading = true;
      this.mainPageActive = false;
      this.settingsActive = false;
      this.aboutActive = false;
      this.bulkActive = false;
      this.app_model.freshUseCaseQueries(catusecases);
      this.$refs.resultComp.reset();
      this.resultActive = true;

      if (this.app_model.queries[0].type == 0) {
        this.$refs.resultComp.search();
      } else if (this.app_model.queries[0].type == 1) {
        this.$refs.resultComp.mapFilter();
      }

      this.searchLoading = false;

    }

  },
  mounted() {

    //this.search();
    window.addEventListener("popstate", this.popStateChange);

    //TODO workaround better to use window resize listener to handle this.
    if (window.innerWidth < 500) {
      this.topSearchBoxSize = 15;
      this.app_conf.page_size = 9;
      this.app_conf.page_size_new = 9;
      this.searchPlaceHolder = "Search";
      this.mobile = true;
    } else if (window.innerWidth <= 1500) {
      this.topSearchBoxSize = 35;
      this.app_conf.page_size = 18;
      this.app_conf.page_size_new = 18;
    }
  },
  beforeMount() {
    let search = document.location.search;

    if (search.length > 2) {
      this.searchTerm = decodeURIComponent(search.substring(1, search.length));
      this.searchNoHistory();
    }

    //set this app to model for event handlings
    this.app_model.setAppComp(this);
  }
};
</script>

<style>
#app {
  /** font-family: 'Avenir', Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  text-align: center;
  color: #2c3e50;
  margin-top: 60px; **/
  display: flex;
  min-height: 100vh;
  flex-direction: column;
}

.logoColor {
  color: #c0c0c0;
}

.logoColor2 {
  color: darkturquoise;
}

.mainContent {
  flex: 1;
}

::-webkit-input-placeholder {
  color: peachpuff;
  font-size: 19px;
}
::-moz-placeholder {
  color: blue;
  font-size: 19px;
}
:-ms-input-placeholder {
  color: peachpuff;
  font-size: 19px;
}
::placeholder {
  color: peachpuff;
  font-size: 19px;
}

.notificationBar {
  margin: 1px;
  margin-bottom: 0;
}

.notification.n-light {
  margin: 10px;
  margin-bottom: 0;
  border-radius: 3px;
  font-size: 13px;
  padding: 10px 20px;
  color: #495061;
  background: #eaf4fe;
  border: 1px solid #d4e8fd;
}
.notification-title {
  letter-spacing: 1px;
  text-transform: uppercase;
  font-size: 10px;
  color: #2589f3;
}

.resultDivOdd {
  background-color: white;
  /**border-bottom:1px solid red;**/
  padding: 2px;
}

.resultDivEven {
  background-color: #f2f2f2;
  /**border-bottom:1px solid red;**/
  padding: 2px;
}

.resultContainer {
  margin-top: 10px;
}

.fa_custom {
  color: #0099cc;
}

p.tree,
ul.tree,
ul.tree ul {
  list-style: none;
  margin: 0;
  padding: 0;
}

ul.tree ul {
  margin-left: 1em;
}

ul.tree li {
  margin-left: 0.35em;
  border-left: thin solid #000;
}

ul.tree li:last-child {
  border-left: none;
}

ul.tree li:before {
  width: 0.9em;
  height: 0.6em;
  margin-right: 0.1em;
  vertical-align: top;
  border-bottom: thin solid #000;
  content: "";
  display: inline-block;
}

ul.tree li:last-child:before {
  border-left: thin solid #000;
}

.pageTitle {
  color: firebrick;
  padding: 12px;
}

.resultTitle {
  text-align: center;
  vertical-align: middle;
  color: firebrick;
  font-weight: bold;
  padding-right: 85px;
}
</style>