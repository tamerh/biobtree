export default class XrefModel {

    constructor(_fetcher, _xref_conf, _app_conf) {
        this.fetcher = _fetcher;
        this.xref_conf = _xref_conf;
        this.app_conf = _app_conf;
        this.app_comp = null;
        this.all_sub_results = [];
        this.all_map_results = [];
        this.queries = [];
        this.result_counter = 0;
        this.hasGlobalFilter = this.app_conf.global_filter_datasets && this.app_conf.global_filter_datasets.length > 0;
    }

    freshSearchQuery(searchTerm) {

        this.reset();
        this.newQuery(0, searchTerm, "", "");

    }

    freshMapFilterQuery(searchTerm, mapFilterTerm) {

        this.reset();
        this.newQuery(1, searchTerm, mapFilterTerm, "");
    }

    freshUseCaseQueries(usecases) {

        this.reset();

        for (let i = 0; i < usecases.length; i++) {
            const usecase = usecases[i];
            if (usecase.type == 0) {
                this.newQuery(0, usecase.searchTerm, "", usecase.name, usecase.source, usecase.source);
            } else if (usecase.type == 1) {
                this.newQuery(1, usecase.searchTerm, usecase.mapFilterTerm, usecase.name, usecase.source, usecase.source);
            }
        }

    }

    reset() {
        this.all_sub_results.splice(0, this.all_sub_results.length);
        this.all_map_results.splice(0, this.all_map_results.length);
        this.queries.splice(0, this.queries.length);
    }


    newQuery(type, searchTerm, mapFilterTerm, namee, source, sourceName) {

        if (!source) {
            source = "";
        }

        if (!sourceName) {
            sourceName = "";
        }

        this.queries.push({
            name: namee,
            type: type,
            mapFilterTerm: mapFilterTerm,
            searchTerm: searchTerm,
            restURL: "",
            nextPageKey: "",
            filterActive: false,
            filter: "",
            edit: false,
            loading: false,
            attributes: true,
            retrieved: false,
            showDatasets: false,
            selectedDataset: source,
            selectedDatasetName: sourceName
        });

        if (type == 1) {
            this.all_map_results.push([]);
        } else if (type == 0) {
            this.all_sub_results.push([]);
        }

    }
    deleteQuery(index) {

        if (this.queries[index].type == 0) {
            this.all_sub_results.splice(index - this.previousMapQueryCount(index), 1);
        } else if (this.queries[index].type == 1) {
            this.all_map_results.splice(index - this.previousSearchQueryCount(index), 1);
        }
        this.queries.splice(index, 1);

    }

    previousMapQueryCount(index) {
        let previousMapQueryCount = 0;
        for (let i = index - 1; i >= 0; i--) {
            if (this.queries[i].type == 1) {
                previousMapQueryCount++;
            }
        }
        return previousMapQueryCount;
    }

    previousSearchQueryCount(index) {
        let previousSearchQueryCount = 0;
        for (let i = index - 1; i >= 0; i--) {
            if (this.queries[i].type == 0) {
                previousSearchQueryCount++;
            }
        }
        return previousSearchQueryCount;
    }

    setAppComp(_app_comp) {
        this.app_comp = _app_comp;
    }

    setGlobHasFilter(hasFilter) {
        this.hasGlobalFilter = hasFilter;
    }

    mapFilter(searchTerm, mapFilterTerm) {

        if (searchTerm.startsWith("alias:")) {
            var alias = searchTerm.split("alias:")[1];
            if (alias.length <= 1) {
                this.app_comp.notifyUser(0, "Alias input length must be greater than 1");
            }
            this.fetcher.mapFilter(searchTerm, alias, mapFilterTerm, this.processMPResults.bind(this))
        } else {
            this.fetcher.mapFilter(searchTerm, "", mapFilterTerm, this.processMPResults.bind(this))
        }


    }

    clearResults() {
        this.all_sub_results = [];
        this.all_map_results = [];
    }
    processMPResults(resp, queryIndex) {

        let resultIndex = queryIndex - this.previousSearchQueryCount(queryIndex);

        if (resp == null) {
            this.app_comp.notifyUser(0, "No mapping found");
            this.all_map_results.splice(resultIndex, 1, []); //stay at the same page
            return "";
        } else if (resp.Err != null) {
            resp.Err = resp.Err.replace("<input>", "input")
            this.app_comp.notifyUser(0, resp.Err);
            this.all_map_results.splice(resultIndex, 1, []);
            return "";
        } else if (!resp.results || resp.results.length == 0 || resp.results[0] == null) {
            this.app_comp.notifyUser(0, "No mapping found");
            this.all_map_results.splice(resultIndex, 1, []);
            return "";
        } else if (!resp.results[0].targets) {
            this.app_comp.notifyUser(0, "No mapping found");
            this.all_map_results.splice(resultIndex, 1, []);
            return "";
        }

        this.all_map_results.splice(resultIndex, 1, resp.results); // this way is needed to notify the vue about this change

        if (resp.nextpage && resp.nextpage.length > 0) {
            return resp.nextpage;
        }

        return "";

    }

    processResults(data_results, queryIndex) {

        queryIndex = queryIndex - this.previousMapQueryCount(queryIndex);

        this.all_sub_results[queryIndex].length = 0;

        if (data_results == null) {
            this.app_comp.notifyUser(0, "No result found");
            this.all_sub_results.splice(queryIndex, 1, []);
            return "";
        } else if (data_results.Err != null) {
            data_results.Err = data_results.Err.replace("<input>", "input")
            this.app_comp.notifyUser(0, data_results.Err);
            this.all_sub_results.splice(queryIndex, 1, []);
            return "";
        } else if (data_results[0] && data_results[0].Err != null) {
            data_results[0].Err = data_results[0].Err.replace("<input>", "input")
            this.app_comp.notifyUser(0, data_results[0].Err);
            this.all_sub_results.splice(queryIndex, 1, []);
            return "";
        }

        let results = data_results.results;

        for (let key2 in results) {
            this.prepareResult(results[key2], 0);
            if (!this.hasGlobalFilter) { // otherwise we should set after applying filter
                this.addResult(results[key2], queryIndex);
            }
        }


        this.app_comp.searchLoading = false;

        if (data_results.nextpage && data_results.nextpage.length > 0) {
            return data_results.nextpage;
        }

        return "";

    }

    addResult(result, queryIndex) {
        let ix = 0;
        while (ix < this.all_sub_results[queryIndex].length) {
            if (result.count > this.all_sub_results[queryIndex][ix].count) {
                break;
            }
            ix++;
        }
        this.all_sub_results[queryIndex].splice(ix, 0, result);
    }


    prepareResult(result, depth) {

        this.result_counter++;
        result.counter = this.result_counter;
        result.showResults = true;
        result.filterModalActive = false;
        result.treeModal = false;
        result.selectedXrefs = [];
        result.displayEntries = [];
        result.depth = depth;
        // check the labels

        this.preparePaging(result);

        this.prepareFilter(result);

        this.applyGlobFilter(result);
    }

    prepareFilter(result) {

        let domain_counts = result.dataset_counts;
        //first sort by count
        domain_counts.sort(function (a, b) {
            if (a.count < b.count) return 1;
            if (a.count > b.count) return -1;
            return 0;
        });
        for (let key3 in domain_counts) {
            let domain_count = domain_counts[key3];
            domain_count.selected = true;
            try {
                domain_count.filterLabel = this.xref_conf[domain_count.dataset].name + '(' + domain_count.count.toLocaleString() + ')';
            } catch (e) {
                domain_count.filterLabel = domain_count.dataset;
            }
        }

    }

    preparePaging(result) {

        result.clientPage = 0;
        result.maxClientPage = 0;
        result.serverPage = 0;
        result.maxServerPage = 0;

        if (result.hasFilter) { //if filter active total count is equal to selected ones
            let filter_total = 0;
            for (let key in result.dataset_counts) {
                let domain_count = result.dataset_counts[key];
                if (domain_count.selected) {
                    filter_total += domain_count.count;
                }
            }
            result.count = filter_total;
        }

        if (result.count > this.app_conf.page_size) {

            result.maxClientPage = Math.ceil(result.count / this.app_conf.page_size) - 1;

            if (result.count > this.app_conf.server_result_page_size) {
                result.maxServerPage = Math.ceil(result.count / this.app_conf.server_result_page_size);
            }

            result.displayEntries = result.entries.slice(0, this.app_conf.page_size);

        } else {

            result.displayEntries = result.entries;

        }

        this.prepareEntries(result);
    }

    prepareEntries(result) {

        for (let key in result.entries) {

            let entry = result.entries[key];

            if (entry.identifier.length <= 12) {
                entry.label = entry.identifier;
                entry.title = '';
            } else {
                entry.label = entry.identifier.substring(0, 10) + '...';
                entry.title = entry.identifier;
            }

            entry.style = {
                'background-color': this.app_conf.box_color
            }
        }

    }

    resetResult(result, result_org) {

        this.prepareEntries(result.entries);
        result_org.entries = result.entries;
        result_org.count = result.count
        this.prepareResult(result_org, null, 0);

    }

    applyGlobFilter(result) {

        if (this.hasGlobalFilter) {

            let datasets = this.app_conf.global_filter_datasets;
            let domain_counts = result.dataset_counts;
            //first unselect all 
            for (let key in domain_counts) {
                domain_counts[key].selected = false;
            }

            let found = false;
            for (let index = 0; index < datasets.length; index++) {
                const element = datasets[index];
                for (let key2 in domain_counts) {
                    if (domain_counts[key2].dataset === element.id) {
                        found = true;
                        domain_counts[key2].selected = true;
                    }
                }
            }
            if (!found) {

                //TODO set displayed entries empty
                if (result.depth == 0) {
                    this.addResult(result);
                }
                result.count = 0;
                result.displayEntries = [];

            } else {

                let filters = '';
                for (var key in domain_counts) {
                    let domain_count = domain_counts[key];
                    if (domain_count.selected) {
                        filters += domain_count.dataset + ',';
                    }
                }

                result.filters = filters;
                result.hasFilter = true;
                result.lastFilteredPageKey = null;

                this.fetcher.searchByFilter(
                    result,
                    this.processGlobalFilteredResults.bind(this),
                    []
                );

            }

        }

    }

    processSelectedXref(results, callback_params) {

        if (results.length > 0) {
            let sub_result = results[0]
            this.prepareResult(sub_result, callback_params[4].depth + 1);
            callback_params[4].selectedXrefs.unshift(sub_result);
            callback_params[5].selected = true;
            callback_params[5].style["background-color"] = this.app_conf.selected_box_color;
            return sub_result;
        }

    }

    processGlobalFilteredResults(data_results, result, fromPaging) {

        this.processFilteredResults(data_results, result, false);

        if (result.depth == 0) {
            this.addResult(result);
        }

    }
    processFilteredResults(data_results, result, fromPaging) {


        this.prepareEntries(data_results[0].results[0]);

        if (fromPaging) {
            Array.prototype.push.apply(result.entries, data_results[0].results[0].entries);
        } else {
            result.entries = data_results[0].results[0].entries;
            this.preparePaging(result);
        }

        result.lastFilteredPageKey = data_results[0].results[0].identifier; //this is a bit magic for now. we are getting pageindex from identifer field
    }

    processPagingResults(results, result_org) {

        result_org.serverPage++;

        for (let key in results) {
            let result = results[key];
            for (let key2 in result.results) {
                let sub_result = result.results[key2];
                if (sub_result.identifier === result_org.identifier && sub_result.dataset === result_org.dataset) {
                    // now add all the result entries to existing entries
                    //eclipse issue
                    //sub_result_org.entries.push(...sub_result.entries);
                    this.prepareEntries(sub_result);
                    Array.prototype.push.apply(result_org.entries, sub_result.entries);
                }
            }
        }

    }

    resetPaging() {

        for (let key in this.all_sub_results) {
            this.preparePaging(this.all_sub_results[key]);
        }

    }

    resetBoxColors() {

        for (let key in this.all_sub_results) {
            let sub_result = this.all_sub_results[key];

            changeColors(sub_result, this.app_conf);

            change_all_sub_entries(sub_result, this.app_conf);

        }

        function change_all_sub_entries(sub_result, app_conf) {

            for (let key in sub_result.selectedXrefs) {
                let sel_sub_result = sub_result.selectedXrefs[key];
                changeColors(sel_sub_result, app_conf);
                change_all_sub_entries(sel_sub_result, app_conf);
            }
        }

        function changeColors(sub_result, app_conf) {

            for (let key3 in sub_result.entries) {
                let entry = sub_result.entries[key3];
                if (entry.selected) {
                    entry.style["background-color"] = app_conf.selected_box_color;
                } else {
                    entry.style["background-color"] = app_conf.box_color;
                }
            }
        }

    }

}