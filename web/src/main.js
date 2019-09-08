// The Vue build version to load with the `import` command
// (runtime-only or standalone) has been set in webpack.base.conf with an alias.
import Vue from 'vue'
import App from './App'
import Notifications from 'vue-notification'
import VueScrollTo from 'vue-scrollto'
import Multiselect from 'vue-multiselect'

Vue.config.productionTip = false

import Fetch from './data/Fetch.js'
import Model from './data/Model.js'
import AppConf from './conf/AppConf.js'
import UseCases from './conf/UseCases.js'

Vue.use(Notifications)


Vue.use(VueScrollTo, {
    container: "body",
    duration: 500,
    easing: "ease",
    offset: -50,
    cancelable: true,
    onStart: false,
    onDone: false,
    onCancel: false,
    x: false,
    y: true
})

Vue.component('multiselect', Multiselect)

/* eslint-disable no-new */
new Vue({
    el: '#app',
    data() {
        return {
            fetcher: null,
            xref_conf: null,
            model: null,
            app_conf: null,
            usecases: null
        }
    },
    components: {
        App
    },
    template: '<App :xref_conf="this.xref_conf" :app_conf="this.app_conf" :app_model="this.model" :usecases="this.usecases"/>',
    beforeMount() {
        var endpoint = "http://localhost:8888/ws/";
        //var endpoint = "https://www.ebi.ac.uk/~tgur/xrefmap/api.php?ids=";
        //var endpoint = "http://----:8888/ws/"

        this.fetcher = new Fetch(endpoint)
        var request = new XMLHttpRequest();
        request.open('GET', endpoint + 'meta/', false); // `false` makes the request synchronous
        request.send(null);

        if (request.status === 200) {
            this.xref_conf = JSON.parse(request.responseText)
            // todo this is workaround for now
            this.xref_conf[2].bacteriaUrl = "http://bacteria.ensembl.org/Multi/Search/Results?species=all;idx=;q=£{id};;site=ensemblunit"
            this.xref_conf[2].fungiUrl = "http://fungi.ensembl.org/Multi/Search/Results?species=all;idx=;q=£{id};;site=ensemblunit"
            this.xref_conf[2].metazoaUrl = "http://metazoa.ensembl.org/Multi/Search/Results?species=all;idx=;q=£{id};;site=ensemblunit"
            this.xref_conf[2].plantsUrl = "http://plants.ensembl.org/Multi/Search/Results?species=all;idx=;q=£{id};;site=ensemblunit"
            this.xref_conf[2].protistsUrl = "http://protists.ensembl.org/Multi/Search/Results?species=all;idx=;q=£{id};;site=ensemblunit"
        }

        this.app_conf = AppConf;
        this.usecases = UseCases;
        this.model = new Model(this.fetcher, this.xref_conf, this.app_conf);
    }
})