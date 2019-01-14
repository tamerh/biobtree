<template>
  <div id="app">

    <header>

      <nav
        class="navbar has-shadow"
        id="topnav"
      >
        <div class="container">
          <div class="navbar-brand">
            <div class="navbar-item">
              <h1
                v-show="!mainPageActive"
                class="title is-size-4 logoColor"
                @click="goToMain()"
                style="padding-bottom:10px;cursor:pointer"
              >biobtree</h1>
            </div>

            <div
              class="navbar-burger burger"
              :class="{ 'navbar-burger':true, 'burger':true}"
              @click="burgerBarActive = !burgerBarActive"
              data-target="navMenu"
            ><span></span><span></span><span></span></div>
          </div>

          <div class="navbar-start">

            <div class="navbar-item">

              <div
                class="control has-icons-left"
                v-show="!mainPageActive"
              >
                <p class="control is-expanded">
                  <input
                    class="input is-medium"
                    ref="searchbox"
                    type="search"
                    placeholder="Search"
                    :size="topSearchBoxSize"
                    v-model="searchTerm"
                    v-on:keyup.enter="search"
                    @keyup="searchKeyUp"
                    maxlength="300"
                  />
                  <span class="icon is-medium is-left"><i class="fa fa-search"></i></span>
                </p>
              </div>
            </div>

          </div>
          <div
            :class="{'navbar-menu':true,'is-active':burgerBarActive}"
            style="margin-right:-0.5rem"
          >
            <div class="navbar-end">
              <a
                class="navbar-item"
                @click="goToAbout"
              >About</a>

              <a
                class="navbar-item"
                @click="goToBulk"
              >Bulk Query</a>
              <a
                class="navbar-item"
                @click="goToSettings"
              >Settings</a>
              <!--           <a class="navbar-item" @click="apiActive=true">API</a> -->

            </div>
          </div>

        </div>
      </nav>
    </header>

    <main class="mainContent">

      <!-- Hero content: will be in the middle -->
      <div
        class="hero-body is-fullheight"
        v-show="mainPageActive"
      >
        <div class="container has-text-centered">
          <h1
            class="title is-size-1 logoColor"
            style="padding-bottom:10px;"
          >biobtree</h1>
          <h2 class="subtitle"></h2>

          <div class="columns is-gapless">
            <div class="column"></div>
            <div class="column is-four-fifths">
              <div class="control has-icons-left has-icons-right">
                <p class="control is-expanded">
                  <input
                    class="input is-large"
                    type="search"
                    :placeholder="searchPlaceHolder"
                    v-model="searchTerm"
                    v-on:keyup.enter="search"
                    @blur="showExample=true"
                    @keyup="searchKeyUp"
                    maxlength="300"
                    autofocus
                  />
                  <span class="icon is-medium is-left"><i class="fa fa-search"></i></span>
                </p>
              </div>
            </div>
            <div class="column"></div>
          </div>
          <br />
          <br />
          <div>
            <p
              class="title is-size-4"
              style="padding-bottom:5px"
            >Search examples</p>
          </div>

          <div class="field is-grouped is-grouped-centered">
            <p class="control">
              <a @click="exampleQuery('brca2,tpi1')">Gene symbols </a>
            </p>
          </div>

          <div class="field is-grouped is-grouped-centered">
            <p class="control">
              <a @click="exampleQuery('Homo Sapiens,Arabidopsis thaliana')">Species name</a>
            </p>
          </div>

          <div class="field is-grouped is-grouped-centered">

            <p class="control">
              <a @click="exampleQuery('vav_human,tpis_mouse,Q29371')">Uniprot accessions and identifiers</a>
            </p>
          </div>

          <div class="field is-grouped is-grouped-centered">

            <p class="control">
              <a @click="exampleQuery('ENSG00000141968')">Ensembl identifier</a>
            </p>
          </div>

          <div class="field is-grouped is-grouped-centered">

            <p class="control">
              <a @click="exampleQuery('AC020895,NP_005419.2')">Sequence identifiers</a>
            </p>
          </div>

          <div class="field is-grouped is-grouped-centered">

            <p class="control">
              <a @click="exampleQuery('hsa:7409')">KEGG identifier</a>
            </p>
          </div>

          <div class="field is-grouped is-grouped-centered">

            <p class="control">
              <a @click="exampleQuery('1.1.1.44')">Enzymes</a>
            </p>
          </div>

          <div class="field is-grouped is-grouped-centered">
            <p class="control">
              <a @click="exampleQuery('CHEBI:17790')">Chemical identifiers</a>
            </p>
          </div>

          <div class="field is-grouped is-grouped-centered">
            <p class="control">
              <a @click="exampleQuery('GO:0008081')">Ontology identifier</a>
            </p>
          </div>

          <div class="field is-grouped is-grouped-centered">
            <p class="control">
              <a @click="exampleQuery('28472374,10.1038/NATURE12211')">Literature,DOI identifiers</a>
            </p>
          </div>

          <!--<div class="field is-grouped is-grouped-centered">
            <p class="control">
              <a @click="exampleQuery('FIG|1348657.5.PEG.3738')">PATRIC identifier</a>
            </p>
          </div>

          <div class="field is-grouped is-grouped-centered">
            <p class="control">
              <a @click="exampleQuery('VGNC:10496')">VGNC identifier</a>
            </p>
          </div>-->

        </div>
      </div>

      <bulk
        :bulkActive="bulkActive"
        :xref_conf="xref_conf"
        v-on:close-bulk="bulkActive=false"
      />

      <about
        :aboutActive="aboutActive"
        v-on:close-about="aboutActive=false"
      />
      <api
        :apiActive="apiActive"
        v-on:close-api="apiActive=false"
      />
      <settings
        :settingsActive="settingsActive"
        :app_conf="app_conf"
        :xref_conf="xref_conf"
        v-on:apply-settings="applySettings()"
      />

      <div
        v-for="(sub_res,index) in app_model.all_sub_results"
        :class="resultDivClass(index)"
      >
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

      <notifications
        group="xrefmap"
        position="top center"
        classes="notification is-warning"
      />
      <notifications
        group="error"
        position="top center"
        classes="notification is-danger"
      />
      <notifications
        group="success"
        position="top center"
        classes="notification is-success"
      />

    </main>

    <footer>
      <div class="content has-text-left is-size-6">
        <!-- <p>
          Last update date: 8 October 2018
        </p> -->
      </div>
    </footer>

  </div>
</template>
  
<script>
import About from "./components/About.vue";
import Bulk from "./components/Bulk.vue";
import Api from "./components/Api.vue";
import Settings from "./components/Settings.vue";
import BoxView from "./components/BoxView.vue";

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
    }
  },
  components: {
    "box-view": BoxView,
    about: About,
    bulk: Bulk,
    api: Api,
    settings: Settings
  },
  data() {
    return {
      // searchTerm: 'brca2,tpi1,rs1,homo sapiens,219572,vav_human,tpis_mouse,Q29371,PRJEB3215,NP_005419.2,1.1.1.44,1.11.1.6,GO:0008081,28472374,10.1038/NATURE12211,JP2006507841',
      //searchTerm: 'UNIREF50_P18031,9606,G8H4L5_9HIV1,',
      //searchTerm:'ENSACAG00000012216',
      searchTerm: "",
      showExample: true,
      aboutActive: false,
      bulkActive: false,
      settingsActive: false,
      apiActive: false,
      mainPageActive: true,
      topSearchBoxSize: 70,
      burgerBarActive: false,
      searchPlaceHolder:
        "Search identifier,accession, hgnc gene symbol or species name seperated by comma",
      mobile: false
    };
  },
  methods: {
    goToMain: function() {
      this.searchTerm = "";
      this.settingsActive = false;
      this.aboutActive = false;
      this.mainPageActive = true;
      this.bulkActive = false;
      this.app_model.all_sub_results = [];
      history.pushState("", "page", "./?m");
    },
    goToBulk: function() {
      this.searchTerm = "";
      this.mainPageActive = false;
      this.settingsActive = false;
      this.aboutActive = false;
      this.bulkActive = true;
      this.app_model.all_sub_results = [];
      history.pushState("", "page", "./?b");
    },
    goToAbout: function() {
      this.searchTerm = "";
      this.mainPageActive = false;
      this.settingsActive = false;
      this.aboutActive = true;
      this.bulkActive = false;
      this.app_model.all_sub_results = [];
      history.pushState("", "page", "./?a");
    },
    goToSettings: function() {
      this.searchTerm = "";
      this.mainPageActive = false;
      this.aboutActive = false;
      this.settingsActive = true;
      this.bulkActive = false;
      this.app_model.all_sub_results = [];
      history.pushState("", "page", "./?s");
    },

    resultReady: function(status) {
      switch (status) {
        case -1:
          this.$notify({
            group: "error",
            title: "",
            text: "Something went wrong. Please try again later."
          });
          break;
        case 0:
          this.$notify({
            group: "xrefmap",
            title: "",
            text: "No result found."
          });
          break;
        default:
          break;
      }
    },
    search: function() {
      if (this.validQuery()) {
        history.pushState(
          "",
          "page",
          "./?" + encodeURIComponent(this.searchTerm)
        );

        this.app_model.search(this.searchTerm);

        this.mainPageActive = false;
        this.settingsActive = false;
        this.aboutActive = false;
        this.bulkActive = false;
        this.$refs.searchbox.blur();
      }
    },
    searchNoHistory: function() {
      if (this.validQuery()) {
        this.app_model.search(this.searchTerm);
        this.mainPageActive = false;
      }
    },
    validQuery: function() {
      if (this.searchTerm.length == 0) {
        return false;
      }
      if (this.searchTerm.length == 1) {
        this.$notify({
          group: "xrefmap",
          title: "",
          text: "Query must be at least 2 characters"
        });
        return false;
      }
      return true;
    },
    resultDivClass: function(index) {
      if (index % 2 == 0) {
        return "resultDivOdd";
      } else {
        return "resultDivEven";
      }
    },
    searchKeyUp: function() {
      if (this.searchTerm.length > 0) {
        this.showExample = false;
      } else {
        this.showExample = true;
      }
    },
    popStateChange: function(e) {
      let search = document.location.search;
      if (search.length > 2) {
        this.searchTerm = decodeURIComponent(
          search.substring(1, search.length)
        );
        this.searchNoHistory();
        this.mainPageActive = false;
        this.settingsActive = false;
        this.aboutActive = false;
        this.bulkActive = false;
      } else if (search === "?m") {
        this.searchTerm = "";
        this.app_model.all_sub_results = [];
        this.mainPageActive = true;
        this.settingsActive = false;
        this.aboutActive = false;
        this.bulkActive = false;
      } else if (search === "?a") {
        this.searchTerm = "";
        this.app_model.all_sub_results = [];
        this.mainPageActive = false;
        this.settingsActive = false;
        this.aboutActive = true;
      } else if (search === "?s") {
        this.searchTerm = "";
        this.app_model.all_sub_results = [];
        this.mainPageActive = false;
        this.settingsActive = true;
        this.aboutActive = false;
        this.bulkActive = false;
      } else if (search === "?b") {
        this.searchTerm = "";
        this.app_model.all_sub_results = [];
        this.mainPageActive = false;
        this.settingsActive = false;
        this.aboutActive = false;
        this.bulkActive = true;
      }
    },
    applySettings: function() {
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

      this.$notify({
        group: "success",
        title: "",
        text: "Settings applied."
      });
    },
    exampleQuery: function(query) {
      this.searchTerm = query;
      this.search();
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
  color: deepskyblue;
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
  background-color: #f8f8f8;
  /**border-bottom:1px solid red;**/
  padding: 2px;
}

.resultDivEven {
  background-color: #eeeeee;
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

a[target^="_blank"]:after {
  /** content: url(http://upload.wikimedia.org/wikipedia/commons/6/64/Icon_External_Link.png);**/
  content: url(data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAYAAACNMs+9AAAAVklEQVR4Xn3PgQkAMQhDUXfqTu7kTtkpd5RA8AInfArtQ2iRXFWT2QedAfttj2FsPIOE1eCOlEuoWWjgzYaB/IkeGOrxXhqB+uA9Bfcm0lAZuh+YIeAD+cAqSz4kCMUAAAAASUVORK5CYII=);
  margin: 0 0 0 1px;
}
</style>
